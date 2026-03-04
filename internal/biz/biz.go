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

type Transaction interface {
	Transaction(ctx context.Context, f func(ctx context.Context) error) error
}
