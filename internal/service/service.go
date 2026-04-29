// TODO(0): remember to do idempotency check for all APIs.
package service

import (
	"azushop/internal/common"

	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ProviderSet is service providers.

func convertToUniquePaths(updateMask *fieldmaskpb.FieldMask) []string {
	ss := common.NewStringSet(common.WithValues(updateMask.GetPaths()))
	return ss.ToSlice()
}
