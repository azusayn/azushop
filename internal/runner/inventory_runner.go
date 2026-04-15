package runner

import (
	"azushop/internal/biz"
	"context"

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
		return r.uc.HandleProductCreated(ctx)
	})
	g.Go(func() error {
		return r.uc.HandleOrderCreated(ctx)
	})
	g.Go(func() error {
		return r.uc.HandlePaymentStatus(ctx)
	})
	return g.Wait()
}

func (r *InventoryRunner) Stop(ctx context.Context) error {
	return nil
}
