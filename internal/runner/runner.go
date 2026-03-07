package runner

import (
	"azushop/internal/biz"
	"context"

	"github.com/google/wire"
	"golang.org/x/sync/errgroup"
)

var ProviderSet = wire.NewSet(
	NewRunnerManager,
)

type Runner interface {
	Run(ctx context.Context) error
}

type RunnerManager struct {
	runners []Runner
	cancel  context.CancelFunc
}

func NewRunnerManager(
	orderUsecase *biz.OrderUsecase,
	inventoryUsecase *biz.InventoryUsecase,
	delayMsgRelayUsecase *biz.DelayMsgRealyUsecase,
) *RunnerManager {
	return &RunnerManager{
		runners: []Runner{
			// NewOrderRunner(orderUsecase),
			// NewInventoryRunner(inventoryUsecase),
			// NewDelayMsgRelayRunner(delayMsgRelayUsecase),
		},
	}
}

func (r *RunnerManager) Start(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	ctx, r.cancel = context.WithCancel(ctx)
	for _, runner := range r.runners {
		localRunner := runner
		g.Go(func() error {
			return localRunner.Run(ctx)
		})
	}
	return g.Wait()
}

func (r *RunnerManager) Stop(ctx context.Context) error {
	if r.cancel != nil {
		r.cancel()
	}
	return nil
}
