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
	"github.com/azusayn/azushop/internal/pkg/grpcclient"
	paymentv1connect "github.com/azusayn/azushop/proto/api/payment/v1/v1connect"
	"github.com/azusayn/azushop/proto/conf"
	"github.com/dnwe/otelsarama"
	"github.com/google/wire"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"gorm.io/gorm"
)

const (
	createPaymentIdempotencyCacheTime = time.Minute * 15
)

var PaymentDataProviderSet = wire.NewSet(
	db.NewPostgres,
	db.NewTransaction,
	cache.NewRedis,
	NewPaymentRepo,
	NewKafkaProducer,
	NewPaymentPublisher,
	NewOrderClient,
)

type PaymentRepo struct {
	postgres *db.Postgres
	redis    *cache.Redis
}

func NewPaymentRepo(postgres *db.Postgres, redis *cache.Redis) biz.PaymentRepo {
	return &PaymentRepo{
		postgres: postgres,
		redis:    redis,
	}
}

func NewPaymentClient(config *conf.Data, propagator propagation.TextMapPropagator) (paymentv1connect.PaymentServiceClient, error) {
	opts, err := grpcclient.NewAuthenticatedClientOptions(propagator)
	if err != nil {
		return nil, err
	}
	return grpcclient.NewServiceClient(
		paymentv1connect.NewPaymentServiceClient,
		config.GetServiceAddr().GetPayment(),
		opts,
	), nil
}

func (repo *PaymentRepo) CreatePayment(
	ctx context.Context,
	idempotencyKey string,
	orderID int64,
	userID int32,
	total decimal.Decimal,
	method biz.PaymentMethod,
	status biz.PaymentStatus,
	externalID string,
) (*biz.Payment, error) {
	var client *gorm.DB
	if client = db.GetTransaction(ctx); client == nil {
		client = repo.postgres.GormClient.WithContext(ctx)
	}

	if _, ok := cache.GetCache[int](ctx, repo.redis, idempotencyKey); ok {
		return nil, errors.New("duplicate payments")
	}

	payment := &biz.Payment{
		ExternalID:  externalID,
		OrderID:     orderID,
		UserID:      userID,
		Method:      method,
		Status:      status,
		AmountTotal: total,
	}
	if err := client.Create(payment).Error; err != nil {
		return nil, err
	}

	cache.SetCache(ctx, repo.redis, idempotencyKey, idempotencyKeyExist, createPaymentIdempotencyCacheTime)

	return payment, nil
}

func (repo *PaymentRepo) UpdatePaymentStatusByOrderID(ctx context.Context, orderID int64, status biz.PaymentStatus) error {
	client := db.GetTransaction(ctx)
	if client == nil {
		client = repo.postgres.GormClient.WithContext(ctx)
	}
	return client.
		WithContext(ctx).
		Model(&biz.Payment{}).
		Where("order_id = ?", orderID).
		Where("status = ?", biz.PaymentStatusPending).
		Update("status", status).Error
}

func (repo *PaymentRepo) UpdatePaymentByID(ctx context.Context, payment *biz.Payment, paths []string) error {
	client := db.GetTransaction(ctx)
	if client == nil {
		client = repo.postgres.GormClient.WithContext(ctx)
	}
	m := make(map[string]any, len(paths))
	for _, path := range paths {
		switch path {
		case "status":
			m[path] = payment.Status
		default:
			return fmt.Errorf("invalid update path %q", path)
		}
	}
	return client.Where("id = ?", payment.ID).Updates(m).Error
}

type PaymentPublisher struct {
	kafkaProducer *KafkaProducer
}

func NewPaymentPublisher(producer *KafkaProducer) biz.PaymentPublisher {
	return &PaymentPublisher{kafkaProducer: producer}
}

func (p *PaymentPublisher) PublishPaymentStatus(ctx context.Context, orderID int64, status biz.PaymentStatus) error {
	producer := p.kafkaProducer.syncProducer
	bytes, err := json.Marshal(biz.PaymentStatusMessage{
		OrderID: orderID,
		Status:  status,
	})
	if err != nil {
		return err
	}
	msg := &sarama.ProducerMessage{
		Topic: string(biz.KafkaTopicPaymentStatus),
		Value: sarama.ByteEncoder(bytes),
	}
	otel.GetTextMapPropagator().Inject(ctx, otelsarama.NewProducerMessageCarrier(msg))
	_, _, err = producer.SendMessage(msg)
	return err
}
