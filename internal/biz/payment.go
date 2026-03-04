package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/checkout/session"
)

type PaymentRepo interface {
	CreatePayment(ctx context.Context, orderID int64, userID int32, total decimal.Decimal,
		method PaymentMethod, status PaymentStatus, externalID string) (*Payment, error)
	UpdatePaymentByID(ctx context.Context, payment *Payment, paths []string) error
	UpdatePaymentStatusByOrderID(ctx context.Context, orderID int64, status PaymentStatus) error
}

type PaymentPublisher interface {
	PublishPaymentPaid(ctx context.Context, orderID int64) error
}

type PaymentUsecase struct {
	repo      PaymentRepo
	publisher PaymentPublisher
}

func NewPaymentUsecase(repo PaymentRepo, publisher PaymentPublisher) *PaymentUsecase {
	return &PaymentUsecase{
		repo:      repo,
		publisher: publisher,
	}
}

type PaymentStatus string

const (
	PaymentStatusUnspcified PaymentStatus = "unspecified"
	PaymentStatusPending    PaymentStatus = "pending"
	PaymentStatusCancelled  PaymentStatus = "cancelled"
	PaymentStatusPaid       PaymentStatus = "paid"
	PaymentStatusRefunding  PaymentStatus = "refunding"
	PaymentStatusRefunded   PaymentStatus = "refunded"
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

// return *Payment and payment URL.
func (uc *PaymentUsecase) CreatePayment(
	ctx context.Context,
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
		externalID, url, amountTotal, err = handleStripeCreatePayment(orderID, userID, items, successURL)
		if err != nil {
			return "", err
		}
		total = decimal.NewFromInt(amountTotal).Div(decimal.NewFromInt(100))
	default:
		return "", fmt.Errorf("unsupported method %q", method)
	}
	_, err = uc.repo.CreatePayment(ctx, orderID, userID, total, method, PaymentStatusPending, externalID)
	if err != nil {
		return "", err
	}
	return url, nil
}

func (uc *PaymentUsecase) Callback(ctx context.Context, method PaymentMethod, body []byte) error {
	var orderID int64
	var paymentStatus PaymentStatus
	var err error
	switch method {
	case PaymentMethodStripe:
		orderID, paymentStatus, err = handleStripeCallback(body)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported payment method %q", method)
	}
	if err := uc.repo.UpdatePaymentStatusByOrderID(ctx, orderID, paymentStatus); err != nil {
		return err
	}
	return uc.publisher.PublishPaymentPaid(ctx, orderID)
}

// create a Stripe checkout session and return:
// - PaymentIntent ID
// - URL
// - AmountTotal
func handleStripeCreatePayment(
	orderID int64,
	userID int32,
	items []*PaymentItem,
	successURL string,
) (string, string, int64, error) {
	var lineItemParams []*stripe.CheckoutSessionLineItemParams
	for _, item := range items {
		unitAmount := item.UnitPrice.Mul(decimal.NewFromInt(100)).IntPart()
		metadata := map[string]string{
			"user_id":  strconv.FormatInt(int64(userID), 10),
			"order_id": strconv.FormatInt(orderID, 10),
		}
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
	params := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems:  lineItemParams,
		SuccessURL: stripe.String(successURL),
		// TODO(3): add CancelURL for better UX.
	}
	s, err := session.New(params)
	if err != nil {
		return "", "", 0, err
	}
	if s.PaymentIntent == nil {
		return "", "", 0, errors.New("missing payment intent")
	}
	return s.PaymentIntent.ID, s.URL, s.AmountTotal, nil
}

// processes callback from payment Stripe's server and
// returns order ID and payment status.
func handleStripeCallback(body []byte) (int64, PaymentStatus, error) {
	var event stripe.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return 0, PaymentStatusUnspcified, err
	}
	var checkoutSession stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &checkoutSession); err != nil {
		return 0, PaymentStatusUnspcified, err
	}
	orderIDStr, ok := checkoutSession.Metadata["order_id"]
	if !ok {
		return 0, PaymentStatusUnspcified, errors.New("failed to get order ID")
	}
	orderID, err := strconv.ParseInt(orderIDStr, 10, 64)
	if err != nil {
		return 0, PaymentStatusUnspcified, err
	}
	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted,
		stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded:
		if checkoutSession.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid {
			slog.Warn("order not paid", "order_id", orderID)
			return orderID, PaymentStatusCancelled, nil
		}
		return orderID, PaymentStatusPaid, nil
	case stripe.EventTypeCheckoutSessionAsyncPaymentFailed:
		slog.Warn("async payment failed", "order_id", orderID)
		return orderID, PaymentStatusCancelled, nil
	case stripe.EventTypeCheckoutSessionExpired:
		slog.Warn("checkout session expired", "order_id", orderID)
		return orderID, PaymentStatusCancelled, nil
	default:
	}
	return orderID, PaymentStatusUnspcified, fmt.Errorf("unsupported stripe event type %q", event.Type)
}
