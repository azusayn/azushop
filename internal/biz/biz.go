package biz

import (
	"context"

	"github.com/google/wire"
)

// ProviderSet is biz providers.
var ProviderSet = wire.NewSet(
	NewOrderUsecase,
	NewPaymentUsecase,
	NewDelayMsgRealyUsecase,
)

const (
	KafkaTopicPaymentStatus  = "payment.status"
	KafkaTopicProductCreated = "product.created"
	KafkaTopicOrderCreated   = "order.created"
	// "order.cancelled.delay" is an intermediate topic
	// used by the delay runner to defer delivery to "order.cancelled"
	KafkaTopicOrderCancelledDelay = "order.cancelled.delay"
	KafkaTopicOrderCancelled      = "order.cancelled"
)

type Transaction interface {
	Transaction(ctx context.Context, f func(ctx context.Context) error) error
}
