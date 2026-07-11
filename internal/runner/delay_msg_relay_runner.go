package runner

import (
	"context"

	"github.com/azusayn/azushop/internal/biz"
)

type DelayMsgRelayRunner struct {
	uc *biz.DelayMsgRealyUsecase
}

func NewDelayMsgRelayRunner(uc *biz.DelayMsgRealyUsecase) *DelayMsgRelayRunner {
	return &DelayMsgRelayRunner{uc: uc}
}

func (r *DelayMsgRelayRunner) Start(ctx context.Context) error {
	return r.uc.HandleDelayMessage(ctx)
}

func (r *DelayMsgRelayRunner) Stop(ctx context.Context) error {
	return nil
}
