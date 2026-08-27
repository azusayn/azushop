package server

import (
	"context"
	"log/slog"
	"time"

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
		return r.uc.HandleKafkaMessages(ctx)
	})

	g.Go(func() error {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := r.uc.ProcessOutboxMessages(ctx); err != nil {
					slog.Error("failed to process outbox messages")
				}
			case <-ctx.Done():
				slog.InfoContext(ctx, "stop processing outbox messages")
				return nil
			}
		}
	})

	return g.Wait()
}

func (r *OrderRunner) Stop(ctx context.Context) error {
	return nil
}

var _ Server = (*OrderRunner)(nil)
