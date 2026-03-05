package service

import (
	"azushop/internal/common"

	"github.com/google/wire"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ProviderSet is service providers.
var ProviderSet = wire.NewSet(
	NewAuthServiceService,
	NewProductService,
	NewInventoryService,
	NewOrderService,
	NewPaymentService,
)

func convertToUniquePaths(updateMask *fieldmaskpb.FieldMask) []string {
	ss := common.NewStringSet(common.WithValues(updateMask.GetPaths()))
	return ss.ToSlice()
}
