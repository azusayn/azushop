package server

import (
	"context"

	"github.com/azusayn/azushop/internal/biz"
)

type ProductRunner struct {
	uc *biz.ProductUsecase
}

func NewProductRunner(uc *biz.ProductUsecase) *ProductRunner {
	return &ProductRunner{uc: uc}
}

func (r *ProductRunner) Start(ctx context.Context) error {
	return r.uc.HandleOutboxMessages(ctx)
}

func (r *ProductRunner) Stop(ctx context.Context) error {
	return nil
}

var _ Server = (*ProductRunner)(nil)
