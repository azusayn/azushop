package server

import (
	"context"

	"github.com/azusayn/azushop/internal/biz"
)

type PaymentRunner struct {
	uc *biz.PaymentUsecase
}

func NewPaymentRunner(uc *biz.PaymentUsecase) *PaymentRunner {
	return &PaymentRunner{uc: uc}
}

func (r *PaymentRunner) Start(ctx context.Context) error {
	return r.uc.HandleOutboxMessages(ctx)
}

func (r *PaymentRunner) Stop(ctx context.Context) error {
	return nil
}

var _ Server = (*PaymentRunner)(nil)
