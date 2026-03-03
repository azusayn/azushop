package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/checkout/session"
)

type PaymentRepo interface {
	CreatePayment(ctx context.Context, orderID int64, userID int32, total decimal.Decimal,
		method PaymentMethod, status PaymentStatus, externalID string) (*Payment, error)
	UpdatePayment(ctx context.Context, payment *Payment, paths []string) error
}

type PaymentUsecase struct {
	repo PaymentRepo
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
		externalID, url, amountTotal, err = handleStripePayment(userID, items, successURL)
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

// create a Stripe checkout session and return:
// - PaymentIntent ID
// - URL
// - AmountTotal
func handleStripePayment(
	userID int32,
	items []*PaymentItem,
	successURL string,
) (string, string, int64, error) {
	var lineItemParams []*stripe.CheckoutSessionLineItemParams
	for _, item := range items {
		unitAmount := item.UnitPrice.Mul(decimal.NewFromInt(100)).IntPart()
		attr := map[string]string{
			"user_id": strconv.Itoa(int(userID)),
		}
		if err := json.Unmarshal(item.Attr, &attr); err != nil {
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
			Metadata: attr,
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
