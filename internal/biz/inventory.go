package biz

import (
	"context"
	"encoding/json"
)

type InventoryRepo interface {
	CreateSKU(ctx context.Context, productID int64, skus []*Sku) error
	UpdateSKU(ctx context.Context, skus []*Sku) error
	ListSKUs(ctx context.Context, productID int64) ([]*Sku, error)
}

type Sku struct {
	ID               int64           `gorm:"column:id"`
	ProductID        int64           `gorm:"column:product_id"`
	Attrs            json.RawMessage `gorm:"column:product_id"`
	StockQuantity    int64           `gorm:"column:stock_quantity"`
	ReservedQuantity int64           `gorm:"column:reserved_quantity"`
	UnitPrice        string          `gorm:"column:unit_price"`
}

type InventoryUsecase struct {
	repo InventoryRepo
}

func NewInventoryUsecase(repo InventoryRepo) *InventoryUsecase {
	return &InventoryUsecase{repo: repo}
}

func (uc *InventoryUsecase) CreateSKU(ctx context.Context, productID int64, skus []*Sku) error {
	return uc.repo.CreateSKU(ctx, productID, skus)
}

func (uc *InventoryUsecase) UpdateSKU(ctx context.Context, skus []*Sku) error {
	return uc.repo.UpdateSKU(ctx, skus)
}

func (uc *InventoryUsecase) ListSKUs(ctx context.Context, productID int64) ([]*Sku, error) {
	return uc.repo.ListSKUs(ctx, productID)
}
