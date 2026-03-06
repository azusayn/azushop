package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/multierr"
)

const (
	OutboxBatchSize int = 100
)

type OrderRepo interface {
	// orders
	ListOrders(ctx context.Context, userID int32, status OrderStatus, pageToken int64, pageSize int32) ([]*Order, error)
	GetOrder(ctx context.Context, orderID int64) (*Order, error)
	CreateOrder(ctx context.Context, orderItems []*OrderItem, total decimal.Decimal, status OrderStatus, userID int32) (*Order, error)
	UpdateOrderStatus(ctx context.Context, orderID int64, status OrderStatus) error
	DeleteOrder(ctx context.Context, orderID int64) error
	CancelOrder(ctx context.Context, orderID int64) error
	// order_outbox
	CreateOutboxMessage(ctx context.Context, topic string, payload json.RawMessage) error
	ListOutboxMessages(ctx context.Context, topic string, limit int) ([]*OrderOutboxMessage, error)
	MarkOutboxMessagesSent(ctx context.Context, ids []uuid.UUID) error
	MarkOutboxMessagesFailed(ctx context.Context, ids []uuid.UUID) error
}
type OrderSubscriber interface {
	SubscribePaymentStatus(ctx context.Context, handler func(orderID int64, status PaymentStatus) error) error
}

type OrderPublisher interface {
	PublishOrderCreated(ctx context.Context, messages []*OrderOutboxMessage) error
}

type OrderUsecase struct {
	repo       OrderRepo
	tx         Transaction
	subscriber OrderSubscriber
	publisher  OrderPublisher
}

func NewOrderUsecase(
	repo OrderRepo,
	subscriber OrderSubscriber,
	publisher OrderPublisher,
	tx Transaction,
) *OrderUsecase {
	return &OrderUsecase{
		repo:       repo,
		tx:         tx,
		subscriber: subscriber,
		publisher:  publisher,
	}
}

type OrderStatus string

const (
	OrderStatusUnspcified OrderStatus = "unspecified"
	OrderStatusPending    OrderStatus = "pending"
	OrderStatusCancelled  OrderStatus = "cancelled"
	OrderStatusConfirmed  OrderStatus = "confirmed"
	OrderStatusCompleted  OrderStatus = "completed"
)

type OrderItem struct {
	ProductName string
	SkuID       uuid.UUID
	Quantity    int64
	UnitPrice   decimal.Decimal
	Attrs       json.RawMessage
}

type Order struct {
	ID     int64           `gorm:"column:id"`
	UserID int32           `gorm:"column:user_id"`
	Total  decimal.Decimal `gorm:"column:total"`
	Status OrderStatus     `gorm:"column:status"`
	// []*OrderItem
	OrderItems json.RawMessage `gorm:"column:order_items"`
	CreatedAt  time.Time       `gorm:"column:created_at"`
}

type OrderOutboxMessage struct {
	ID        uuid.UUID       `gorm:"column:id"`
	Topic     string          `gorm:"column:topic"`
	Payload   json.RawMessage `gorm:"column:payload"`
	CreatedAt time.Time       `gorm:"column:created_at"`
	SentAt    time.Time       `gorm:"column:sent_at"`
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
		total = total.Add(orderItem.UnitPrice.Mul(quantity))
	}
	var createdOrder *Order
	var err error
	err = uc.tx.Transaction(ctx, func(ctx context.Context) error {
		createdOrder, err = uc.repo.CreateOrder(ctx, orderItems, total, OrderStatusPending, userID)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(createdOrder)
		if err != nil {
			return err
		}
		return uc.repo.CreateOutboxMessage(ctx, KafkaTopicOrderCreated, payload)
	})
	if err != nil {
		return nil, err
	}
	return createdOrder, nil
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

func (uc *OrderUsecase) HandlePaymentStatus(ctx context.Context) error {
	return uc.subscriber.SubscribePaymentStatus(ctx, func(orderID int64, status PaymentStatus) error {
		switch status {
		case PaymentStatusPaid,
			PaymentStatusCancelled:
		default:
			return fmt.Errorf("invalid status %q", status)
		}
		return uc.repo.UpdateOrderStatus(ctx, orderID, OrderStatusConfirmed)
	})
}

func (uc *OrderUsecase) ProcessOutboxMessages(ctx context.Context, topic string) error {
	messages, err := uc.repo.ListOutboxMessages(ctx, topic, OutboxBatchSize)
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	switch topic {
	case KafkaTopicOrderCreated:
		err = uc.publisher.PublishOrderCreated(ctx, messages)
	default:
		return fmt.Errorf("unsupported topic %q", topic)
	}
	if err != nil {
		return multierr.Append(err, uc.repo.MarkOutboxMessagesFailed(ctx, ids))
	}
	return uc.repo.MarkOutboxMessagesSent(ctx, ids)
}
