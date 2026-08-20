package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	inventorypb "github.com/azusayn/azushop/proto/api/inventory/v1"
	inventoryv1connect "github.com/azusayn/azushop/proto/api/inventory/v1/v1connect"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	OutboxBatchSize int = 100
)

type OrderRepo interface {
	// orders
	ListOrders(ctx context.Context, userID int32, status OrderStatus, pageToken int64, pageSize int32) ([]*Order, error)
	GetOrder(ctx context.Context, orderID int64) (*Order, error)
	CreateOrder(ctx context.Context, idempotencyKey string, orderItems []*OrderItem, total decimal.Decimal, status OrderStatus, userID int32) (*Order, error)
	UpdateOrderStatus(ctx context.Context, orderID int64, status OrderStatus) error
	DeleteOrder(ctx context.Context, orderID int64) error
	CancelOrder(ctx context.Context, orderID int64) error
	// order_outbox
	CreateOutboxMessage(ctx context.Context, eventType OutboxEventType, payload json.RawMessage) error
	ListOutboxMessages(ctx context.Context, limit int) ([]*OrderOutboxMessage, error)
	MarkOutboxMessagesSent(ctx context.Context, ids []uuid.UUID) error
	MarkOutboxMessagesFailed(ctx context.Context, ids []uuid.UUID) error
}
type OrderSubscriber interface {
	SubscribePaymentStatus(ctx context.Context, handler func(orderID int64, status PaymentStatus) error) error
	SubscribeOrderCancelled(context.Context, func(int64) error) error
}

type OrderPublisher interface {
	PublishOrderCreated(ctx context.Context, messages []*OrderOutboxMessage) error
	PublishOrderCancelledDelay(ctx context.Context, messages []*OrderOutboxMessage) error
}

type OrderUsecase struct {
	repo       OrderRepo
	tx         Transaction
	subscriber OrderSubscriber
	publisher  OrderPublisher
	inventory  inventoryv1connect.InventoryServiceClient
}

func NewOrderUsecase(
	repo OrderRepo,
	subscriber OrderSubscriber,
	publisher OrderPublisher,
	tx Transaction,
	inventory inventoryv1connect.InventoryServiceClient,
) *OrderUsecase {
	return &OrderUsecase{
		repo:       repo,
		tx:         tx,
		subscriber: subscriber,
		publisher:  publisher,
		inventory:  inventory,
	}
}

type OrderStatus string

const (
	OrderStatusUnspecified OrderStatus = "unspecified"
	OrderStatusPending     OrderStatus = "pending"
	OrderStatusCancelled   OrderStatus = "cancelled"
	OrderStatusConfirmed   OrderStatus = "confirmed"
	OrderStatusCompleted   OrderStatus = "completed"
)

type OrderItem struct {
	ProductName string
	SkuID       uuid.UUID
	Quantity    int64
	UnitPrice   decimal.Decimal
	Attrs       json.RawMessage
}

type Order struct {
	ID             int64           `gorm:"column:id"`
	IdempotencyKey string          `gorm:"column:idempotency_key"`
	UserID         int32           `gorm:"column:user_id"`
	Total          decimal.Decimal `gorm:"column:total"`
	Status         OrderStatus     `gorm:"column:status"`
	OrderItems     json.RawMessage `gorm:"column:order_items"`
	CreatedAt      time.Time       `gorm:"column:created_at"`
}

type OrderOutboxMessage struct {
	ID         uuid.UUID       `gorm:"column:id"`
	EventType  OutboxEventType `gorm:"column:event_type"`
	Payload    json.RawMessage `gorm:"column:payload"`
	RetryCount int             `gorm:"retry_count"`
	CreatedAt  time.Time       `gorm:"column:created_at"`
	SentAt     time.Time       `gorm:"column:sent_at"`
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
	idempotencyKey string,
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
		createdOrder, err = uc.repo.CreateOrder(ctx, idempotencyKey, orderItems, total, OrderStatusPending, userID)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(createdOrder)
		if err != nil {
			return err
		}
		err = uc.repo.CreateOutboxMessage(ctx, OutboxEventOrderCreated, payload)
		if err != nil {
			return err
		}
		return uc.repo.CreateOutboxMessage(ctx, OutboxEventOrderCancelledDelay, payload)
	})
	if err != nil {
		return nil, err
	}
	return createdOrder, nil
}

func (uc *OrderUsecase) CancelOrder(ctx context.Context, orderID int64) error {
	return uc.tx.Transaction(ctx, func(ctx context.Context) error {
		if err := uc.repo.CancelOrder(ctx, orderID); err != nil {
			return err
		}
		msg := ReleaseStockMessage{
			OrderID: orderID,
		}
		bytes, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		return uc.repo.CreateOutboxMessage(ctx, OutboxEventOrderCancelled, bytes)
	})
}

func (uc *OrderUsecase) DeleteOrder(ctx context.Context, orderID int64) error {
	return uc.repo.DeleteOrder(ctx, orderID)
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

func (uc *OrderUsecase) HandleOrderCancelled(ctx context.Context) error {
	return uc.subscriber.SubscribeOrderCancelled(ctx, func(orderID int64) error {
		order, err := uc.repo.GetOrder(ctx, orderID)
		if err != nil {
			return err
		}
		if order.Status != OrderStatusPending {
			return nil
		}
		return uc.repo.UpdateOrderStatus(ctx, orderID, OrderStatusCancelled)
	})
}

func (uc *OrderUsecase) ProcessOutboxMessages(ctx context.Context) error {
	messages, err := uc.repo.ListOutboxMessages(ctx, OutboxBatchSize)
	if err != nil {
		return err
	}
	successIDs := make([]uuid.UUID, 0)
	failIDs := make([]uuid.UUID, 0)

	// TODO(3): buffer optimization
	for _, message := range messages {
		if err := uc.dispatchOutboxMessage(ctx, message); err != nil {
			slog.ErrorContext(ctx, "failed to send outbox message", slog.Any("msg", message))
			failIDs = append(failIDs, message.ID)
			continue
		}
		successIDs = append(successIDs, message.ID)
	}

	if err = uc.repo.MarkOutboxMessagesFailed(ctx, failIDs); err != nil {
		err = errors.Join(err, errors.New("failed to mark outbox messages failed"))
	}

	return errors.Join(err, uc.repo.MarkOutboxMessagesSent(ctx, successIDs))
}

// dispatchOutboxMessages dispatches an outbox message to the appropriate handler
// based on its event type.
func (uc *OrderUsecase) dispatchOutboxMessage(ctx context.Context, message *OrderOutboxMessage) error {
	eventType := message.EventType
	switch eventType {
	case OutboxEventOrderCreated:
		return uc.publisher.PublishOrderCreated(ctx, []*OrderOutboxMessage{message})

	case OutboxEventOrderCancelledDelay:
		return uc.publisher.PublishOrderCancelledDelay(ctx, []*OrderOutboxMessage{message})

	case OutboxEventOrderCancelled:
		var msg ReleaseStockMessage
		if err := json.Unmarshal(message.Payload, &msg); err != nil {
			return err
		}
		req := connect.NewRequest(&inventorypb.ReleaseStockRequest{
			OrderId: msg.OrderID,
		})
		_, err := uc.inventory.ReleaseStock(ctx, req)
		return err

	default:
	}

	return fmt.Errorf("unknown event type %q", eventType)
}
