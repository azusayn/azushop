package server

import (
	"context"

	"github.com/azusayn/azushop/internal/biz"
	"golang.org/x/sync/errgroup"
)

type InventoryRunner struct {
	uc *biz.InventoryUsecase
}

func NewInventoryRunner(uc *biz.InventoryUsecase) *InventoryRunner {
	return &InventoryRunner{uc: uc}
}

func (r *InventoryRunner) Start(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return r.uc.HandleKafkaMessages(ctx)
	})

	g.Go(func() error {
		return r.uc.HandleOutboxMessages(ctx)
	})

	return g.Wait()
}

func (r *InventoryRunner) Stop(ctx context.Context) error {
	return nil
}

var _ Server = (*InventoryRunner)(nil)
