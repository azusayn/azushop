package biz

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type OrderRepo interface {
	ListOrders(ctx context.Context, userID int32, status OrderStatus, pageToken int64, pageSize int32) ([]*Order, error)
	CreateOrder(ctx context.Context, orderItems []*OrderItem, total decimal.Decimal, status OrderStatus, userID int32) (*Order, error)
	DeleteOrder(ctx context.Context, orderID int64) error
	CancelOrder(ctx context.Context, orderID int64) error
}

type OrderUsecase struct {
	repo OrderRepo
}

func NewOrderUsecase(repo OrderRepo) *OrderUsecase {
	return &OrderUsecase{repo: repo}
}

type OrderStatus string

const (
	OrderStatusUnspcified OrderStatus = "unspecified"
	OrderStatusPending    OrderStatus = "pending"
	OrderStatusPaid       OrderStatus = "paid"
	OrderStatusCancelled  OrderStatus = "cancelled"
	OrderStatusRefunded   OrderStatus = "refunded"
)

type OrderItem struct {
	SkuID     uuid.UUID
	Quantity  int64
	UnitPrice decimal.Decimal
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
