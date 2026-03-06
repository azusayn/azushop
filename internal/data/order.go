package data

import (
	"azushop/internal/biz"
	"context"
	"encoding/json"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	maxRetryCount = 5
)

type OrderRepo struct {
	data *Data
}

func NewOrderRepo(data *Data) biz.OrderRepo {
	return &OrderRepo{data: data}
}

func (repo *OrderRepo) ListOrders(
	ctx context.Context,
	userID int32,
	status biz.OrderStatus,
	pageToken int64,
	pageSize int32,
) ([]*biz.Order, error) {
	client := repo.data.gormClient
	var orders []*biz.Order
	client = client.WithContext(ctx).Where("user_id = ?", userID).Where("id > ?", pageToken)
	if status != biz.OrderStatusUnspcified {
		client = client.Where("status = ?", status)
	}
	if err := client.Limit(int(pageSize)).Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

func (repo *OrderRepo) CreateOrder(
	ctx context.Context,
	orderItems []*biz.OrderItem,
	total decimal.Decimal,
	orderStatus biz.OrderStatus,
	userID int32,
) (*biz.Order, error) {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.data.gormClient
	}
	itemsJson, err := json.Marshal(orderItems)
	if err != nil {
		return nil, err
	}
	order := &biz.Order{
		UserID:     userID,
		Status:     orderStatus,
		OrderItems: itemsJson,
		Total:      total,
	}
	if err := client.WithContext(ctx).Create(order).Error; err != nil {
		return nil, err
	}
	return order, nil
}

func (repo *OrderRepo) DeleteOrder(ctx context.Context, orderID int64) error {
	gormClient := GetTransaction(ctx)
	if gormClient == nil {
		gormClient = repo.data.gormClient
	}
	return gormClient.WithContext(ctx).Where("id = ?", orderID).Delete(&biz.Order{}).Error
}

func (repo *OrderRepo) CancelOrder(ctx context.Context, orderID int64) error {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.data.gormClient
	}
	return client.WithContext(ctx).Where("id = ?", orderID).Update("status", biz.OrderStatusCancelled).Error
}

func (repo *OrderRepo) GetOrder(ctx context.Context, orderID int64) (*biz.Order, error) {
	client := repo.data.gormClient
	var order biz.Order
	if err := client.WithContext(ctx).Where("id = ?", orderID).Find(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

func (repo *OrderRepo) UpdateOrderStatus(ctx context.Context, orderID int64, status biz.OrderStatus) error {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.data.gormClient
	}
	return client.WithContext(ctx).Where("id = ?", orderID).Update("status", status).Error
}

func (repo *OrderRepo) CreateOutboxMessage(ctx context.Context, topic string, payload json.RawMessage) error {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.data.gormClient
	}
	outboxMsg := &biz.OrderOutboxMessage{
		Topic:   topic,
		Payload: payload,
	}
	return client.WithContext(ctx).Create(outboxMsg).Error
}

// returns messages that are eligible for processing.
func (repo *OrderRepo) ListOutboxMessages(ctx context.Context, topic string, limit int) ([]*biz.OrderOutboxMessage, error) {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.data.gormClient
	}
	// TODO(4): composite index?
	var messages []*biz.OrderOutboxMessage
	if err := client.
		WithContext(ctx).
		Where("sent_at IS NULL").
		Where("topic = ?", topic).
		Where("retry_count < ?", maxRetryCount).
		Order("created_at").
		Limit(limit).
		Find(&messages).
		Error; err != nil {
		return nil, err
	}
	return messages, nil
}

func (repo *OrderRepo) MarkOutboxMessagesSent(ctx context.Context, ids []uuid.UUID) error {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.data.gormClient
	}
	return client.WithContext(ctx).Where("id IN ?", ids).Update("sent_at", time.Now()).Error
}

// increments the retry count by 1 for the given messageIDs.
func (repo *OrderRepo) MarkOutboxMessagesFailed(ctx context.Context, ids []uuid.UUID) error {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.data.gormClient
	}
	return client.WithContext(ctx).Where("id IN ?", ids).Update("retry_count", gorm.Expr("retry_count + 1")).Error
}

type OrderSubscriber struct {
	data *Data
}

func NewOrderSubscriber(data *Data) biz.OrderSubscriber {
	return &OrderSubscriber{data: data}
}

type orderConsumerHandler struct {
	handler func(string) error
}

func (s *OrderSubscriber) SubscribePaymentStatus(ctx context.Context, handler func(int64, biz.PaymentStatus) error) error {
	topics := []string{biz.KafkaTopicPaymentStatus}
	consumerHandler := NewConsumerHandler(func(bytes []byte) error {
		var msg PaymentStatusMessage
		if err := json.Unmarshal(bytes, &msg); err != nil {
			return err
		}
		return handler(msg.OrderID, biz.PaymentStatus(string(msg.Status)))
	})
	for {
		err := s.data.GetPaymentConsumer().Consume(ctx, topics, consumerHandler)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

type OrderPublisher struct {
	data *Data
}

func NewOrderPublisher(data *Data) biz.OrderPublisher {
	return &OrderPublisher{data: data}
}

func (p *OrderPublisher) PublishOrderCreated(ctx context.Context, messages []*biz.OrderOutboxMessage) error {
	producer := p.data.GetKafkaProducer()
	var prodMsgs []*sarama.ProducerMessage
	for _, message := range messages {
		orderCreatedMsg, err := convertToOrderCreatedMessage(message)
		if err != nil {
			return err
		}
		bytes, err := json.Marshal(orderCreatedMsg)
		if err != nil {
			return err
		}
		prodMsg := &sarama.ProducerMessage{
			Topic: biz.KafkaTopicOrderCreated,
			Value: sarama.ByteEncoder(bytes),
		}
		prodMsgs = append(prodMsgs, prodMsg)
	}
	return producer.SendMessages(prodMsgs)
}

func convertToOrderCreatedMessage(message *biz.OrderOutboxMessage) (*OrderCreatedMessage, error) {
	var order biz.Order
	if err := json.Unmarshal(message.Payload, &order); err != nil {
		return nil, err
	}
	var bizOrderItems []*biz.OrderItem
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
