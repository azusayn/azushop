package runner

import (
	"azushop/internal/biz"
	"context"

	"golang.org/x/sync/errgroup"
)

type InventoryRunner struct {
	uc *biz.InventoryUsecase
}

func NewInventoryRunner(uc *biz.InventoryUsecase) Runner {
	return &InventoryRunner{uc: uc}
}

func (r *InventoryRunner) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return r.uc.HandleProductCreated(ctx)
	})
	g.Go(func() error {
		return r.uc.HandleOrderCreated(ctx)
	})
	return g.Wait()
}
