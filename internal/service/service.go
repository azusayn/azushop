// TODO(0): remember to do idempotency check for all APIs.
package service

import (
	"azushop/internal/common"

	"github.com/google/wire"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ProviderSet is service providers.
var ProviderSet = wire.NewSet(
	NewInventoryService,
	NewOrderService,
	NewPaymentService,
)

func convertToUniquePaths(updateMask *fieldmaskpb.FieldMask) []string {
	ss := common.NewStringSet(common.WithValues(updateMask.GetPaths()))
	return ss.ToSlice()
}
