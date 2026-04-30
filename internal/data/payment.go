package data

import (
	"azushop/internal/biz"
	"context"
	"encoding/json"
	"fmt"

	"github.com/IBM/sarama"
	"github.com/google/wire"
	"github.com/shopspring/decimal"
)

var PaymentDataProviderSet = wire.NewSet(
	NewPostgres,
	NewPaymentRepo,
	NewKafkaProducer,
	NewPaymentPublisher,
)

type PaymentRepo struct {
	postgres *Postgres
}

func NewPaymentRepo(postgres *Postgres) biz.PaymentRepo {
	return &PaymentRepo{postgres: postgres}
}

func (repo *PaymentRepo) CreatePayment(
	ctx context.Context,
	orderID int64,
	userID int32,
	total decimal.Decimal,
	method biz.PaymentMethod,
	status biz.PaymentStatus,
	externalID string,
) (*biz.Payment, error) {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.postgres.GormClient.WithContext(ctx)
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
	return payment, nil
}

func (repo *PaymentRepo) UpdatePaymentStatusByOrderID(ctx context.Context, orderID int64, status biz.PaymentStatus) error {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.postgres.GormClient.WithContext(ctx)
	}
	return client.Where("order_id = ?", orderID).Where("status = ?", biz.PaymentStatusPending).Update("status", status).Error
}

func (repo *PaymentRepo) UpdatePaymentByID(ctx context.Context, payment *biz.Payment, paths []string) error {
	client := GetTransaction(ctx)
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
	bytes, err := json.Marshal(PaymentStatusMessage{
		OrderID: orderID,
		Status:  PaymentStatus(string(status)),
	})
	if err != nil {
		return err
	}
	msg := &sarama.ProducerMessage{
		Topic: biz.KafkaTopicPaymentStatus,
		Value: sarama.ByteEncoder(bytes),
	}
	_, _, err = producer.SendMessage(msg)
	return err
}
