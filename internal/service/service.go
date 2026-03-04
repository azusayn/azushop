package service

import "github.com/google/wire"

// ProviderSet is service providers.
var ProviderSet = wire.NewSet(
	NewAuthServiceService,
	NewProductService,
	NewInventoryService,
	NewOrderService,
	NewPaymentService,
)
