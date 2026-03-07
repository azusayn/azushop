package runner

import (
	"azushop/internal/biz"
	"context"

	"golang.org/x/sync/errgroup"
)

type OrderRunner struct {
	uc *biz.OrderUsecase
}

func NewOrderRunner(uc *biz.OrderUsecase) Runner {
	return &OrderRunner{uc: uc}
}

// TODO(3): function for stopping.
func (r *OrderRunner) Run(ctx context.Context) error {
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
