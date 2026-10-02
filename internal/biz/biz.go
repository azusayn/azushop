package biz

import (
	"context"

	"github.com/azusayn/azushop/internal/pkg/kafka"
)

const (
	KafkaTopicPaymentStatus  kafka.TopicType = "payment.status"
	KafkaTopicProductCreated kafka.TopicType = "product.created"
	KafkaTopicOrderCreated   kafka.TopicType = "order.created"
	// "order.cancelled.delay" is an intermediate topic
	// used by the delay runner to defer delivery to "order.cancelled"
	KafkaTopicOrderCancelledDelay kafka.TopicType = "order.cancelled.delay"
	KafkaTopicOrderCancelled      kafka.TopicType = "order.cancelled"
)

type OutboxEventType string

const (
	OutboxEventOrderCreated        OutboxEventType = "outbox.event.order.created"
	OutboxEventOrderCancelled      OutboxEventType = "outbox.event.order.cancelled"
	OutboxEventOrderCancelledDelay OutboxEventType = "outbox.event.order.cancelled_delay"
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
