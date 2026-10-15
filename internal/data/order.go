package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/azusayn/azushop/internal/biz"
	"github.com/azusayn/azushop/internal/pkg/cache"
	"github.com/azusayn/azushop/internal/pkg/db"
	"github.com/azusayn/azushop/internal/pkg/kafka"
	"github.com/azusayn/azushop/proto/conf"
	"github.com/dnwe/otelsarama"
	"github.com/google/wire"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel"
	"gorm.io/gorm"
)

const (
	createOrderIdempotencyCacheTime time.Duration = time.Minute * 10
	idempotencyKeyExist                           = 1
)

var OrderDataProviderSet = wire.NewSet(
	db.NewTransaction,
	db.NewPostgres,
	cache.NewRedis,
	NewOrderRepo,
	NewOrderSubscriber,
	NewKafkaProducer,
	NewOrderPublisher,
	NewProductClient,
	NewInventoryClient,
)

type OrderRepo struct {
	postgres *db.Postgres
	redis    *cache.Redis
}

func NewOrderRepo(postgres *db.Postgres, redis *cache.Redis) biz.OrderRepo {
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
	if _, ok := cache.GetCache[int](ctx, repo.redis, idempotencyKey); ok {
		return nil, errors.New("duplicate orders")
	}

	var client *gorm.DB
	if client = db.GetTransaction(ctx); client == nil {
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

	cache.SetCache(ctx, repo.redis, cacheKey, idempotencyKeyExist, createOrderIdempotencyCacheTime)

	return order, nil
}

func (repo *OrderRepo) DeleteOrder(ctx context.Context, orderID int64) error {
	gormClient := db.GetTransaction(ctx)
	if gormClient == nil {
		gormClient = repo.postgres.GormClient
	}
	return gormClient.WithContext(ctx).Where("id = ?", orderID).Delete(&biz.Order{}).Error
}

func (repo *OrderRepo) CancelOrder(ctx context.Context, orderID int64) error {
	client := db.GetTransaction(ctx)
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
	client := db.GetTransaction(ctx)
	if client == nil {
		client = repo.postgres.GormClient
	}

	return client.
		WithContext(ctx).
		Model(&biz.Order{}).
		Where("id = ?", orderID).
		Update("status", status).Error
}

type OrderSubscriber struct {
	consumerGroup sarama.ConsumerGroup
}

func NewOrderSubscriber(config *conf.Data) (biz.OrderSubscriber, error) {
	consumerGroup, err := kafka.NewConsumerGroup(config.GetKafka().GetBrokerAddrs(), config.GetServiceName())
	if err != nil {
		return nil, err
	}
	return &OrderSubscriber{
		consumerGroup: consumerGroup,
	}, nil
}

func (s *OrderSubscriber) Subscribe(ctx context.Context, handlers map[kafka.TopicType]kafka.HandlerFunc) error {
	consumerHandler := kafka.NewConsumerHandler(handlers)
	topics := make([]string, 0, len(handlers))
	for topic := range handlers {
		topics = append(topics, string(topic))
	}
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

type OrderPublisher struct {
	kafkaProducer *KafkaProducer
}

// TODO: move to pkg.
func NewOrderPublisher(producer *KafkaProducer) biz.OrderPublisher {
	return &OrderPublisher{kafkaProducer: producer}
}

func (p *OrderPublisher) SendMessages(ctx context.Context, messages []*kafka.Message) error {
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
		otel.GetTextMapPropagator().Inject(ctx, otelsarama.NewProducerMessageCarrier(prodMsg))
		prodMsgs = append(prodMsgs, prodMsg)
	}

	if err := producer.SendMessages(prodMsgs); err != nil {
		return fmt.Errorf("failed to send %d message(s)", len(prodMsgs))
	}

	return nil
}
