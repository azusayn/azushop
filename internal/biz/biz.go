package biz

import (
	"context"

	"github.com/google/wire"
)

// ProviderSet is biz providers.
var ProviderSet = wire.NewSet(
	NewUserUsecase,
	NewProductUsecase,
	NewInventoryUsecase,
	NewOrderUsecase,
	NewPaymentUsecase,
)

const (
	KafkaTopicPaymentStatus  = "payment.status"
	KafkaTopicProductCreated = "product.created"
	KafkaTopicOrderCreated   = "order.created"
)

type Transaction interface {
	Transaction(ctx context.Context, f func(ctx context.Context) error) error
}
