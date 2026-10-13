package biz

import (
	"context"

	"github.com/azusayn/azushop/internal/pkg/kafka"
)

const (
	KafkaTopicPaymentStatus  kafka.TopicType = "payment.status"
	KafkaTopicProductCreated kafka.TopicType = "product.created"
	KafkaTopicOrderCreated   kafka.TopicType = "order.created"
	KafkaTopicOrderCancelled kafka.TopicType = "order.cancelled"
)

type OutboxEventType string

const (
	outboxEventOrderCreated        OutboxEventType = "outbox.event.order.created"
	outboxEventOrderCancelled      OutboxEventType = "outbox.event.order.cancelled"
	outboxEventOrderCancelledDelay OutboxEventType = "outbox.event.order.cancelled_delay"
)

// TODO(3): decouple these outgoing message structs from their corresponding outbox payloads.
type PaymentStatusMessage struct {
	OrderID int64
	Status  PaymentStatus
}

type ReleaseStockMessage struct {
	OrderID int64
}

type Transaction interface {
	Transaction(ctx context.Context, f func(ctx context.Context) error) error
}
