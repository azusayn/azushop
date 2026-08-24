package biz

import (
	"context"
)

type KafkaTopicType string

const (
	KafkaTopicPaymentStatus  KafkaTopicType = "payment.status"
	KafkaTopicProductCreated KafkaTopicType = "product.created"
	KafkaTopicOrderCreated   KafkaTopicType = "order.created"
	// "order.cancelled.delay" is an intermediate topic
	// used by the delay runner to defer delivery to "order.cancelled"
	KafkaTopicOrderCancelledDelay KafkaTopicType = "order.cancelled.delay"
	KafkaTopicOrderCancelled      KafkaTopicType = "order.cancelled"
	KafkaTopicRetryQueue          KafkaTopicType = "order.retry_queue"
	KafkaTopicDeadLetterQueue     KafkaTopicType = "order.dead_letter_queue"
)

type KafkaMessage struct {
	Topic string
	Value any
}

type RetryQueueMessage struct {
	EventType  RetryQueueEventType
	RetryCount int
	Message    any
}

type OutboxEventType string

const (
	OutboxEventOrderCreated        OutboxEventType = "outbox.event.order.created"
	OutboxEventOrderCancelled      OutboxEventType = "outbox.event.order.cancelled"
	OutboxEventOrderCancelledDelay OutboxEventType = "outbox.event.order.cancelled_delay"
)

type RetryQueueEventType string

const (
	RetryQueueEventTypeReleaseStock RetryQueueEventType = "mq.retry.event.release_stock"
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
