package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"uuid"

	"connectrpc.com/connect"
	"github.com/azusayn/azushop/internal/pkg/kafka"
	"github.com/azusayn/azushop/internal/pkg/kafka/retry"
	"github.com/azusayn/azushop/internal/pkg/telemetry"
	inventorypb "github.com/azusayn/azushop/proto/api/inventory/v1"
	inventoryv1connect "github.com/azusayn/azushop/proto/api/inventory/v1/v1connect"
	"github.com/azusayn/azushop/proto/conf"
	"github.com/shopspring/decimal"
)

const (
	OutboxBatchSize     int = 100
	OutboxMaxRetryCount int = 5
	OrderTimeout            = time.Second * 30
	maxGoroutines       int = 20

	retryEventReleaseStock = "mq.retry.event.release_stock"
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
	Subscribe(ctx context.Context, handlers map[kafka.TopicType]kafka.HandlerFunc) error
}

type OrderPublisher interface {
	SendMessages(ctx context.Context, messages []*kafka.Message) error
}

type OrderUsecase struct {
	repo       OrderRepo
	tx         Transaction
	subscriber OrderSubscriber
	publisher  OrderPublisher
	inventory  inventoryv1connect.InventoryServiceClient
	retrier    *retry.Retrier
}

func NewOrderUsecase(
	repo OrderRepo,
	subscriber OrderSubscriber,
	publisher OrderPublisher,
	tx Transaction,
	inventory inventoryv1connect.InventoryServiceClient,
	config *conf.Data,
) *OrderUsecase {
	return &OrderUsecase{
		repo:       repo,
		tx:         tx,
		subscriber: subscriber,
		publisher:  publisher,
		inventory:  inventory,
		retrier:    retry.NewRetrier(config.GetAppName(), publisher, subscriber),
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
	RetryCount int             `gorm:"column:retry_count"`
	CreatedAt  time.Time       `gorm:"column:created_at"`
	Headers    json.RawMessage `gorm:"column:headers"`
	SentAt     *time.Time      `gorm:"column:sent_at"`
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

func (uc *OrderUsecase) HandleKafkaMessages(ctx context.Context) error {
	return uc.subscriber.Subscribe(ctx, map[kafka.TopicType]kafka.HandlerFunc{
		KafkaTopicOrderCancelled: uc.handleOrderCancelled,
		KafkaTopicPaymentStatus:  uc.handlePaymentStatus,
	})
}

func (uc *OrderUsecase) HandleRetryMessages(ctx context.Context) error {
	return uc.retrier.Run(ctx)
}

func (uc *OrderUsecase) handlePaymentStatus(ctx context.Context, bytes []byte) error {
	var msg PaymentStatusMessage
	if err := json.Unmarshal(bytes, &msg); err != nil {
		return err
	}
	switch msg.Status {
	case PaymentStatusPaid,
		PaymentStatusCancelled:
	default:
		return fmt.Errorf("invalid status %q", msg.Status)
	}
	return uc.repo.UpdateOrderStatus(ctx, msg.OrderID, OrderStatusConfirmed)
}

func (uc *OrderUsecase) handleOrderCancelled(ctx context.Context, bytes []byte) error {
	var msg OrderCancelledMessage
	if err := json.Unmarshal(bytes, &msg); err != nil {
		return err
	}
	order, err := uc.repo.GetOrder(ctx, msg.OrderID)
	if err != nil {
		return err
	}
	if order.Status != OrderStatusPending {
		return nil
	}
	return uc.repo.UpdateOrderStatus(ctx, msg.OrderID, OrderStatusCancelled)
}

func (uc *OrderUsecase) ProcessOutboxMessages(ctx context.Context) error {
	messages, err := uc.repo.ListOutboxMessages(ctx, OutboxBatchSize)
	if err != nil {
		return err
	}

	sentIDs := make([]uuid.UUID, 0)
	failedIDs := make([]uuid.UUID, 0)
	var mu sync.Mutex

	markID := func(id uuid.UUID, success bool) {
		mu.Lock()
		defer mu.Unlock()
		if success {
			sentIDs = append(sentIDs, id)
			return
		}
		failedIDs = append(failedIDs, id)
	}

	var wg sync.WaitGroup
	limitCh := make(chan struct{}, maxGoroutines)

	for _, message := range messages {
		limitCh <- struct{}{}
		wg.Go(func() {
			defer func() { <-limitCh }()

			msgCtx, err := telemetry.InjectTraceHeaderBytes(ctx, message.Headers)
			if err != nil {
				slog.ErrorContext(ctx, "failed to inject headers into context", slog.Any("message_id", message.ID), slog.Any("err", err))
				markID(message.ID, false)
				return
			}

			if err := uc.dispatchOutboxMessage(msgCtx, message); err != nil {
				slog.ErrorContext(msgCtx, "failed to send outbox message", slog.Any("msg", message), slog.Any("err", err))
				markID(message.ID, false)
				return
			}

			markID(message.ID, true)
		})
	}

	wg.Wait()

	return errors.Join(
		uc.repo.MarkOutboxMessagesFailed(ctx, failedIDs),
		uc.repo.MarkOutboxMessagesSent(ctx, sentIDs),
	)
}

// dispatchOutboxMessages dispatches an outbox message to the appropriate handler
// based on its event type.
func (uc *OrderUsecase) dispatchOutboxMessage(ctx context.Context, message *OrderOutboxMessage) error {
	// TODO(3): batch messages by topic
	eventType := message.EventType
	switch eventType {
	case OutboxEventOrderCreated:
		msg, err := toOrderCreatedMessage(message)
		if err != nil {
			return err
		}
		return uc.publisher.SendMessages(ctx, []*kafka.Message{{
			Topic: KafkaTopicOrderCreated,
			Value: msg,
		}})

	case OutboxEventOrderCancelledDelay:
		msg, err := toOrderCancelledMsg(message)
		if err != nil {
			return err
		}
		return uc.publisher.SendMessages(ctx, []*kafka.Message{{
			Topic: KafkaTopicOrderCancelledDelay,
			Value: msg,
		}})

	case OutboxEventOrderCancelled:
		var msg ReleaseStockMessage
		if err := json.Unmarshal(message.Payload, &msg); err != nil {
			return err
		}
		return uc.releaseStock(ctx, &msg)

	default:
	}

	return fmt.Errorf("unknown event type %q", eventType)
}

func (uc *OrderUsecase) releaseStock(
	ctx context.Context,
	msg *ReleaseStockMessage,
) error {
	retryMsg := &retry.Message{
		EventType: retryEventReleaseStock,
		Message:   msg,
	}

	fn := func(ctx context.Context) error {
		req := connect.NewRequest(&inventorypb.ReleaseStockRequest{
			OrderId: msg.OrderID,
		})
		if _, err := uc.inventory.ReleaseStock(ctx, req); err != nil {
			return fmt.Errorf("failed to release stock for order %d", msg.OrderID)
		}
		return nil
	}

	return uc.retrier.Submit(ctx, retryMsg, fn)
}

type OrderCreatedMessage struct {
	OrderID    int64
	OrderItems []*OrderItem
}

func toOrderCreatedMessage(message *OrderOutboxMessage) (*OrderCreatedMessage, error) {
	var order Order
	if err := json.Unmarshal(message.Payload, &order); err != nil {
		return nil, err
	}
	var bizOrderItems []*OrderItem
	if err := json.Unmarshal(order.OrderItems, &bizOrderItems); err != nil {
		return nil, err
	}
	var orderItems []*OrderItem
	for _, bizOrderItem := range bizOrderItems {
		orderItems = append(orderItems, &OrderItem{
			SkuID:    bizOrderItem.SkuID,
			Quantity: bizOrderItem.Quantity,
		})
	}
	orderCreatedMsg := &OrderCreatedMessage{
		OrderID:    order.ID,
		OrderItems: orderItems,
	}
	return orderCreatedMsg, nil
}

type OrderCancelledMessage struct {
	OrderID     int64
	ExpiredTime time.Time
}

func toOrderCancelledMsg(message *OrderOutboxMessage) (*OrderCancelledMessage, error) {
	var order Order
	if err := json.Unmarshal(message.Payload, &order); err != nil {
		return nil, err
	}
	orderCancelledMsg := &OrderCancelledMessage{
		OrderID:     order.ID,
		ExpiredTime: time.Now().Add(OrderTimeout),
	}
	return orderCancelledMsg, nil
}
