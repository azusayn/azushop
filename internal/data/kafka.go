package data

import (
	"log/slog"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
)

const (
	KafkaTopicPaymentStatus  = "payment.status"
	KafkaTopicProductCreated = "product.created"
	KafkaTopicOrderCreated   = "order.created"
)

type PaymentStatus string

const (
	PaymentStatusUnspcified PaymentStatus = "unspecified"
	PaymentStatusPending    PaymentStatus = "pending"
	PaymentStatusCancelled  PaymentStatus = "cancelled"
	PaymentStatusPaid       PaymentStatus = "paid"
	PaymentStatusRefunding  PaymentStatus = "refunding"
	PaymentStatusRefunded   PaymentStatus = "refunded"
)

type PaymentStatusMessage struct {
	OrderID int64
	Status  PaymentStatus
}

type ProductCreatedMessage struct {
	SkuIDs []uuid.UUID
}

type OrderItem struct {
	SkuID    uuid.UUID
	Quantity int64
}

type OrderCreatedMessage struct {
	OrderID    int64
	OrderItems []*OrderItem
}

type ConsumerHandler struct {
	handler func([]byte) error
}

func NewConsumerHandler(handler func([]byte) error) sarama.ConsumerGroupHandler {
	return &ConsumerHandler{handler: handler}
}

func (c *ConsumerHandler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

func (c *ConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (c *ConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case <-session.Context().Done():
			return nil
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			if err := c.handler(msg.Value); err != nil {
				slog.Warn(err.Error())
			}
			session.MarkMessage(msg, "")
		}
	}
}
