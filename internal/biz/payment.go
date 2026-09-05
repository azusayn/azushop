package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/azusayn/azushop/internal/pkg/telemetry"
	"github.com/azusayn/azushop/proto/conf"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/checkout/session"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const paymentCallbackSpanName = "payment.callback"

type PaymentRepo interface {
	CreatePayment(ctx context.Context, idempotencyKey string, orderID int64, userID int32, total decimal.Decimal,
		method PaymentMethod, status PaymentStatus, externalID string) (*Payment, error)
	UpdatePaymentByID(ctx context.Context, payment *Payment, paths []string) error
	UpdatePaymentStatusByOrderID(ctx context.Context, orderID int64, status PaymentStatus) error
}

type PaymentPublisher interface {
	PublishPaymentStatus(ctx context.Context, orderID int64, status PaymentStatus) error
}

type PaymentUsecase struct {
	repo      PaymentRepo
	publisher PaymentPublisher
	appName   string
}

func NewPaymentUsecase(repo PaymentRepo, publisher PaymentPublisher, config *conf.Data) *PaymentUsecase {
	return &PaymentUsecase{
		repo:      repo,
		publisher: publisher,
		appName:   config.GetAppName(),
	}
}

type PaymentStatus string

const (
	PaymentStatusUnspecified PaymentStatus = "unspecified"
	PaymentStatusPending     PaymentStatus = "pending"
	PaymentStatusCancelled   PaymentStatus = "cancelled"
	PaymentStatusPaid        PaymentStatus = "paid"
	PaymentStatusRefunding   PaymentStatus = "refunding"
	PaymentStatusRefunded    PaymentStatus = "refunded"
)

type PaymentMethod string

const (
	PaymentMethodStripe PaymentMethod = "stripe"
	PaymentMethodAlipay PaymentMethod = "alipay"
	PaymentMethodWechat PaymentMethod = "wechat"
)

type Payment struct {
	ID          int64           `gorm:"column:id"`
	ExternalID  string          `gorm:"column:external_id"`
	OrderID     int64           `gorm:"column:order_id"`
	UserID      int32           `gorm:"column:user_id"`
	Method      PaymentMethod   `gorm:"column:method"`
	Status      PaymentStatus   `gorm:"column:status"`
	AmountTotal decimal.Decimal `gorm:"column:amount_total"`
}

type PaymentItem struct {
	Name      string
	Quantity  int64
	UnitPrice decimal.Decimal
	Attr      json.RawMessage
}

type stripeCallbackResult struct {
	OrderID  int64
	Status   PaymentStatus
	TraceMap map[string]string
}

// CreatePayment creates a payemnt and returns a payment link from the payment provider.
func (uc *PaymentUsecase) CreatePayment(
	ctx context.Context,
	idempotencyKey string,
	orderID int64,
	userID int32,
	method PaymentMethod,
	items []*PaymentItem,
	successURL string,
) (string, error) {
	var (
		url        string
		externalID string
		err        error
		total      decimal.Decimal
	)
	switch method {
	case PaymentMethodStripe:
		var amountTotal int64
		externalID, url, amountTotal, err = createStripePayment(ctx, orderID, userID, items, successURL)
		if err != nil {
			return "", err
		}
		total = decimal.NewFromInt(amountTotal).Div(decimal.NewFromInt(100))
	default:
		return "", fmt.Errorf("unsupported method %q", method)
	}
	_, err = uc.repo.CreatePayment(ctx, idempotencyKey, orderID, userID, total, method, PaymentStatusPending, externalID)
	if err != nil {
		return "", err
	}
	return url, nil
}

func (uc *PaymentUsecase) Callback(ctx context.Context, method PaymentMethod, body []byte) error {
	var (
		orderID       int64
		paymentStatus PaymentStatus
		err           error
	)
	switch method {
	case PaymentMethodStripe:
		var result *stripeCallbackResult
		result, err = handleStripeCallback(body)
		if err != nil {
			return err
		}
		orderID = result.OrderID
		paymentStatus = result.Status
		ctx = telemetry.ContextWithTraceMap(ctx, result.TraceMap)
	default:
		return fmt.Errorf("unsupported payment method %q", method)
	}

	ctx, span := otel.Tracer(uc.appName).Start(ctx, paymentCallbackSpanName)
	defer span.End()

	if err := uc.repo.UpdatePaymentStatusByOrderID(ctx, orderID, paymentStatus); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if err := uc.publisher.PublishPaymentStatus(ctx, orderID, paymentStatus); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// createStripePayment creates a Stripe checkout session and return PaymentIntent ID, URL and AmountTotal
func createStripePayment(
	ctx context.Context,
	orderID int64,
	userID int32,
	items []*PaymentItem,
	successURL string,
) (string, string, int64, error) {
	var lineItemParams []*stripe.CheckoutSessionLineItemParams
	for _, item := range items {
		unitAmount := item.UnitPrice.Mul(decimal.NewFromInt(100)).IntPart()
		metadata := make(map[string]string)
		if err := json.Unmarshal(item.Attr, &metadata); err != nil {
			return "", "", 0, err
		}

		lineItemParams = append(lineItemParams, &stripe.CheckoutSessionLineItemParams{
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				// TODO(3): support different currencies
				Currency: stripe.String("cny"),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name: stripe.String(item.Name),
				},
				UnitAmount: stripe.Int64(unitAmount),
			},
			Quantity: stripe.Int64(item.Quantity),
			Metadata: metadata,
		})
	}

	sessionMetadata := map[string]string{
		"user_id":  strconv.FormatInt(int64(userID), 10),
		"order_id": strconv.FormatInt(orderID, 10),
	}
	for k, v := range telemetry.InjectTraceMap(ctx) {
		sessionMetadata[k] = v
	}

	params := &stripe.CheckoutSessionParams{
		Params: stripe.Params{
			Context: ctx,
		},
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems:  lineItemParams,
		SuccessURL: stripe.String(successURL),
		// TODO(3): add CancelURL for better UX.
		Metadata: sessionMetadata,
	}
	s, err := session.New(params)
	if err != nil {
		return "", "", 0, err
	}

	return s.ID, s.URL, s.AmountTotal, nil
}

// handleStripeCallback processes callback from payment Stripe's server.
func handleStripeCallback(body []byte) (*stripeCallbackResult, error) {
	var event stripe.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, err
	}
	var checkoutSession stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &checkoutSession); err != nil {
		return nil, err
	}
	orderIDStr, ok := checkoutSession.Metadata["order_id"]
	if !ok {
		return nil, errors.New("failed to get order ID")
	}
	orderID, err := strconv.ParseInt(orderIDStr, 10, 64)
	if err != nil {
		return nil, err
	}

	traceMap := make(map[string]string)
	if v, ok := checkoutSession.Metadata["traceparent"]; ok {
		traceMap["traceparent"] = v
	}
	if v, ok := checkoutSession.Metadata["tracestate"]; ok {
		traceMap["tracestate"] = v
	}

	result := &stripeCallbackResult{
		OrderID:  orderID,
		TraceMap: traceMap,
	}
	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted,
		stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded:
		if checkoutSession.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid {
			slog.Warn("order not paid", "order_id", orderID)
			result.Status = PaymentStatusCancelled
			return result, nil
		}
		result.Status = PaymentStatusPaid
		return result, nil
	case stripe.EventTypeCheckoutSessionAsyncPaymentFailed:
		slog.Warn("async payment failed", "order_id", orderID)
		result.Status = PaymentStatusCancelled
		return result, nil
	case stripe.EventTypeCheckoutSessionExpired:
		slog.Warn("checkout session expired", "order_id", orderID)
		result.Status = PaymentStatusCancelled
		return result, nil
	default:
	}
	result.Status = PaymentStatusUnspecified
	return result, fmt.Errorf("unsupported stripe event type %q", event.Type)
}
