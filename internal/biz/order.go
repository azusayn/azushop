package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type OrderRepo interface {
	ListOrders(ctx context.Context, userID int32, status OrderStatus, pageToken int64, pageSize int32) ([]*Order, error)
	GetOrder(ctx context.Context, orderID int64) (*Order, error)
	CreateOrder(ctx context.Context, orderItems []*OrderItem, total decimal.Decimal, status OrderStatus, userID int32) (*Order, error)
	UpdateOrderStatus(ctx context.Context, orderID int64, status OrderStatus) error
	DeleteOrder(ctx context.Context, orderID int64) error
	CancelOrder(ctx context.Context, orderID int64) error
}

type OrderSubscriber interface {
	SubscribePaymentPaid(ctx context.Context, handler func(orderID int64, status PaymentStatus) error) error
}

type OrderUsecase struct {
	repo       OrderRepo
	subscriber OrderSubscriber
}

func NewOrderUsecase(repo OrderRepo) *OrderUsecase {
	return &OrderUsecase{repo: repo}
}

type OrderStatus string

const (
	OrderStatusUnspcified OrderStatus = "unspecified"
	OrderStatusPending    OrderStatus = "pending"
	OrderStatusCancelled  OrderStatus = "cancelled"
	OrderStatusPaid       OrderStatus = "confirmed"
	OrderStatusRefunded   OrderStatus = "completed"
)

type OrderItem struct {
	ProductName string
	SkuID       uuid.UUID
	Quantity    int64
	UnitPrice   decimal.Decimal
	Attrs       json.RawMessage
}

type Order struct {
	ID         int64           `gorm:"column:id"`
	UserID     int32           `gorm:"column:user_id"`
	Total      decimal.Decimal `gorm:"column:total"`
	Status     OrderStatus     `gorm:"column:status"`
	OrderItems json.RawMessage `gorm:"column:order_items"`
	CreatedAt  time.Time       `gorm:"column:created_at"`
}

// retrieves orders by user ID, filtered by order status.
func (uc *OrderUsecase) ListOrders(
	ctx context.Context,
	userID int32,
	status OrderStatus,
	pageToken int64,
	pageSize int32,
) ([]*Order, error) {
	return uc.repo.ListOrders(ctx, userID, status, pageToken, pageSize)
}

func (uc *OrderUsecase) CreateOrder(
	ctx context.Context,
	orderItems []*OrderItem,
	userID int32,
) (*Order, error) {
	var total decimal.Decimal
	for _, orderItem := range orderItems {
		quantity := decimal.NewFromInt(orderItem.Quantity)
		total.Add(orderItem.UnitPrice.Mul(quantity))
	}
	return uc.repo.CreateOrder(ctx, orderItems, total, OrderStatusPending, userID)
}

func (uc *OrderUsecase) CancelOrder(ctx context.Context, orderID int64) error {
	return uc.CancelOrder(ctx, orderID)
}

func (uc *OrderUsecase) DeleteOrder(ctx context.Context, orderID int64) error {
	return uc.DeleteOrder(ctx, orderID)
}

func (uc *OrderUsecase) GetOrder(ctx context.Context, orderID int64) (*Order, error) {
	return uc.repo.GetOrder(ctx, orderID)
}

func (uc *OrderUsecase) HandlePaymentPaid(ctx context.Context) error {
	return uc.subscriber.SubscribePaymentPaid(ctx, func(orderID int64, status PaymentStatus) error {
		switch status {
		case PaymentStatusPaid,
			PaymentStatusCancelled:
		default:
			return fmt.Errorf("invalid status %q", status)
		}
		return uc.repo.UpdateOrderStatus(ctx, orderID, OrderStatusPaid)
	})
}
