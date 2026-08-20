// TODO(0): remember to do idempotency check for all APIs.
package service

import (
	"github.com/azusayn/azushop/internal/pkg/str"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func convertToUniquePaths(updateMask *fieldmaskpb.FieldMask) []string {
	ss := str.NewStringSet(str.WithValues(updateMask.GetPaths()))
	return ss.ToSlice()
}

const (
	ServiceNameAuth      = "service.auth"
	ServiceNameOrder     = "service.order"
	ServiceNameInventory = "service.inventory"
	ServiceNameProduct   = "service.product"
	ServiceNamePayment   = "service.payment"
)
