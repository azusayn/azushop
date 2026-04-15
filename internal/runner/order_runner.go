package runner

import (
	"azushop/internal/biz"
	"context"

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
		return r.uc.HandlePaymentStatus(ctx)
	})
	g.Go(func() error {
		return r.uc.ProcessOutboxMessages(ctx, biz.KafkaTopicOrderCreated)
	})
	g.Go(func() error {
		return r.uc.ProcessOutboxMessages(ctx, biz.KafkaTopicOrderCancelledDelay)
	})
	g.Go(func() error {
		return r.uc.HandleOrderCancelled(ctx)
	})
	return g.Wait()
}

func (r *OrderRunner) Stop(ctx context.Context) error {
	return nil
}
