package server

import (
	"context"

	"github.com/azusayn/azushop/internal/biz"
	"golang.org/x/sync/errgroup"
)

type OrderRunner struct {
	uc *biz.OrderUsecase
}

func NewOrderRunner(uc *biz.OrderUsecase) *OrderRunner {
	return &OrderRunner{uc: uc}
}

func (r *OrderRunner) Start(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return r.uc.HandleRetryMessages(ctx)
	})

	g.Go(func() error {
		return r.uc.HandleKafkaMessages(ctx)
	})

	g.Go(func() error {
		return r.uc.HandleOutboxMessages(ctx)
	})

	return g.Wait()
}

func (r *OrderRunner) Stop(ctx context.Context) error {
	return nil
}

var _ Server = (*OrderRunner)(nil)
