package biz

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type OrderRepo interface {
	CreateOrder(ctx context.Context, orderItems []*OrderItem, userID int32) (*Order, error)
}

type OrderUsecase struct {
	repo OrderRepo
}

func NewOrderUsecase(repo OrderRepo) *OrderUsecase {
	return &OrderUsecase{repo: repo}
}

type OrderStatus string

const (
	OrderStatusUnspcified = "unspecified"
	OrderStatusPending    = "pending"
	OrderStatusPaid       = "paid"
	OrderStatusCancelled  = "cancelled"
	OrderStatusRefunded   = "refunded"
)

type OrderItem struct {
	SkuID     uuid.UUID
	Quantity  int64
	UnitPrice string
}

type Order struct {
	ID         int64           `gorm:"column:id"`
	UserID     int32           `gorm:"column:user_id"`
	Total      string          `gorm:"column:total"`
	Status     string          `gorm:"column:status"`
	OrderItems json.RawMessage `gorm:"column:order_items"`
	CreatedAt  time.Time       `gorm:"column:created_at"`
}

func (uc *OrderUsecase) CreateOrder(
	ctx context.Context,
	orderItems []*OrderItem,
	userID int32,
) (*Order, error) {
	return uc.repo.CreateOrder(ctx, orderItems, userID)
}
