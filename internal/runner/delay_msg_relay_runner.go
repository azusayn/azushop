package runner

import (
	"azushop/internal/biz"
	"context"
)

type DelayMsgRelayRunner struct {
	uc *biz.DelayMsgRealyUsecase
}

func NewDelayMsgRelayRunner(uc *biz.DelayMsgRealyUsecase) Runner {
	return &DelayMsgRelayRunner{uc: uc}
}

func (r *DelayMsgRelayRunner) Run(ctx context.Context) error {
	return r.uc.HandleDelayMessage(ctx)
}
