package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"uuid"

	"github.com/IBM/sarama"
	"github.com/azusayn/azushop/internal/biz"
	"github.com/azusayn/azushop/proto/conf"
	"github.com/google/wire"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

const (
	createOrderIdempotencyCacheTime time.Duration = time.Minute * 10
	idempotencyKeyExist                           = 1
)

var OrderDataProviderSet = wire.NewSet(
	NewTransaction,
	NewPostgres,
	NewRedis,
	NewOrderRepo,
	NewOrderSubscriber,
	NewKafkaProducer,
	NewOrderPublisher,
	NewProductClient,
	NewInventoryClient,
)

type OrderRepo struct {
	postgres *Postgres
	redis    *Redis
}

func NewOrderRepo(postgres *Postgres, redis *Redis) biz.OrderRepo {
	return &OrderRepo{
		postgres: postgres,
		redis:    redis,
	}
}

func (repo *OrderRepo) ListOrders(
	ctx context.Context,
	userID int32,
	status biz.OrderStatus,
	pageToken int64,
	pageSize int32,
) ([]*biz.Order, error) {
	client := repo.postgres.GormClient
	var orders []*biz.Order
	client = client.WithContext(ctx).Where("user_id = ?", userID).Where("id > ?", pageToken)
	if status != biz.OrderStatusUnspecified {
		client = client.Where("status = ?", status)
	}
	if err := client.Limit(int(pageSize)).Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

func cacheKeyIdempotencyKey(key string) string {
	return fmt.Sprintf("idempotency:%s", key)
}

func (repo *OrderRepo) CreateOrder(
	ctx context.Context,
	idempotencyKey string,
	orderItems []*biz.OrderItem,
	total decimal.Decimal,
	orderStatus biz.OrderStatus,
	userID int32,
) (*biz.Order, error) {
	// An idempotency key not found in the cache would be passed to the database.
	cacheKey := cacheKeyIdempotencyKey(idempotencyKey)
	if _, ok := GetCache[int](ctx, repo.redis, idempotencyKey); ok {
		return nil, errors.New("duplicate orders")
	}

	var client *gorm.DB
	if client = GetTransaction(ctx); client == nil {
		client = repo.postgres.GormClient
	}
	itemsJson, err := json.Marshal(orderItems)
	if err != nil {
		return nil, err
	}
	order := &biz.Order{
		IdempotencyKey: idempotencyKey,
		UserID:         userID,
		Status:         orderStatus,
		OrderItems:     itemsJson,
		Total:          total,
	}
	if err := client.WithContext(ctx).Create(order).Error; err != nil {
		return nil, err
	}

	SetCache(ctx, repo.redis, cacheKey, idempotencyKeyExist, createOrderIdempotencyCacheTime)

	return order, nil
}

func (repo *OrderRepo) DeleteOrder(ctx context.Context, orderID int64) error {
	gormClient := GetTransaction(ctx)
	if gormClient == nil {
		gormClient = repo.postgres.GormClient
	}
	return gormClient.WithContext(ctx).Where("id = ?", orderID).Delete(&biz.Order{}).Error
}

func (repo *OrderRepo) CancelOrder(ctx context.Context, orderID int64) error {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.postgres.GormClient
	}

	result := client.
		WithContext(ctx).
		Model(&biz.Order{}).
		Where("id = ?", orderID).
		Where("status IN ?", []biz.OrderStatus{biz.OrderStatusPending, biz.OrderStatusConfirmed}).
		Update("status", biz.OrderStatusCancelled)
	if err := result.Error; err != nil {
		return err
	}

	if result.RowsAffected == 0 {
		return errors.New("order cannot be cancelled")
	}

	return nil
}

func (repo *OrderRepo) GetOrder(ctx context.Context, orderID int64) (*biz.Order, error) {
	client := repo.postgres.GormClient
	var order biz.Order
	if err := client.WithContext(ctx).Where("id = ?", orderID).Find(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

func (repo *OrderRepo) UpdateOrderStatus(ctx context.Context, orderID int64, status biz.OrderStatus) error {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.postgres.GormClient
	}
	return client.WithContext(ctx).Model(&biz.Order{}).Where("id = ?", orderID).Update("status", status).Error
}

func (repo *OrderRepo) CreateOutboxMessage(ctx context.Context, eventType biz.OutboxEventType, payload json.RawMessage) error {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.postgres.GormClient
	}
	id := uuid.NewV7()
	outboxMsg := &biz.OrderOutboxMessage{
		ID:        id,
		EventType: eventType,
		Payload:   payload,
	}
	return client.WithContext(ctx).Create(outboxMsg).Error
}

// returns messages that are eligible for processing.
func (repo *OrderRepo) ListOutboxMessages(ctx context.Context, limit int) ([]*biz.OrderOutboxMessage, error) {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.postgres.GormClient
	}
	// TODO(4): composite index?
	var messages []*biz.OrderOutboxMessage
	if err := client.
		WithContext(ctx).
		Where("sent_at IS NULL").
		Where("retry_count < ?", biz.OutboxMaxRetryCount).
		Order("created_at ASC").
		Limit(limit).
		Find(&messages).
		Error; err != nil {
		return nil, err
	}
	return messages, nil
}

func (repo *OrderRepo) MarkOutboxMessagesSent(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.postgres.GormClient
	}
	if err := client.
		WithContext(ctx).
		Model(&biz.OrderOutboxMessage{}).
		Where("id IN ?", ids).
		Update("sent_at", time.Now()).
		Error; err != nil {
		return fmt.Errorf("failed to mark outbox message (%v) sent: %w", ids, err)
	}

	return nil
}

func (repo *OrderRepo) MarkOutboxMessagesFailed(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.postgres.GormClient
	}

	if err := client.
		WithContext(ctx).
		Model(&biz.OrderOutboxMessage{}).
		Where("id IN ?", ids).
		Update("retry_count", gorm.Expr("retry_count + 1")).
		Error; err != nil {
		return fmt.Errorf("failed to mark outbox message (%v) failed: %w", ids, err)
	}

	return nil
}

type OrderSubscriber struct {
	handlers      map[string]func(context.Context, []byte) error
	consumerGroup sarama.ConsumerGroup
}

func NewOrderSubscriber(config *conf.Data) (biz.OrderSubscriber, error) {
	brokerAddrs := config.GetKafka().GetBrokerAddrs()
	consumerGroup, err := NewConsumerGroup(brokerAddrs, config.GetAppName())
	if err != nil {
		return nil, err
	}
	return &OrderSubscriber{
		handlers:      make(map[string]func(context.Context, []byte) error),
		consumerGroup: consumerGroup,
	}, nil
}

func (s *OrderSubscriber) Subscribe(ctx context.Context) error {
	subscribe := func(topics []string) error {
		consumerHandler := NewConsumerHandler(s.handlers)
		for {
			err := s.consumerGroup.Consume(ctx, topics, consumerHandler)
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	}

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return subscribe(lo.Keys(s.handlers))
	})
	g.Go(func() error {
		return subscribe([]string{string(biz.KafkaTopicRetryQueue)})
	})

	return g.Wait()
}

func (s *OrderSubscriber) RegisterHandler(topic biz.KafkaTopicType, handler func(context.Context, []byte) error) {
	s.handlers[string(topic)] = handler
}

type OrderPublisher struct {
	kafkaProducer *KafkaProducer
}

func NewOrderPublisher(producer *KafkaProducer) biz.OrderPublisher {
	return &OrderPublisher{kafkaProducer: producer}
}

func (p *OrderPublisher) SendMessagaes(ctx context.Context, messages []*biz.KafkaMessage) error {
	producer := p.kafkaProducer.syncProducer
	var prodMsgs []*sarama.ProducerMessage
	for _, message := range messages {
		bytes, err := json.Marshal(message.Value)
		if err != nil {
			return err
		}
		prodMsg := &sarama.ProducerMessage{
			Topic: string(message.Topic),
			Value: sarama.ByteEncoder(bytes),
		}
		prodMsgs = append(prodMsgs, prodMsg)
	}

	if err := producer.SendMessages(prodMsgs); err != nil {
		return fmt.Errorf("failed to send %d message(s)", len(prodMsgs))
	}

	return nil
}
