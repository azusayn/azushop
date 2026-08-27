package server

import (
	"context"

	"github.com/azusayn/azushop/internal/biz"
)

type InventoryRunner struct {
	uc *biz.InventoryUsecase
}

func NewInventoryRunner(uc *biz.InventoryUsecase) *InventoryRunner {
	return &InventoryRunner{uc: uc}
}

func (r *InventoryRunner) Start(ctx context.Context) error {
	return r.uc.HandleKafkaMessages(ctx)
}

func (r *InventoryRunner) Stop(ctx context.Context) error {
	return nil
}

var _ Server = (*InventoryRunner)(nil)
