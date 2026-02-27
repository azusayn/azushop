package biz

import (
	"context"
)

type InventoryRepo interface {
	CreateSKU(ctx context.Context, productID int64, skus []*Inventory) error
	UpdateSKU(ctx context.Context, skus []*Inventory) error
	ListSKUs(ctx context.Context, productID int64) ([]*Inventory, error)
}

type Inventory struct {
	ID               int64 `gorm:"column:id"`
	ProductID        int64 `gorm:"column:product_id"`
	StockQuantity    int64 `gorm:"column:stock_quantity"`
	ReservedQuantity int64 `gorm:"column:reserved_quantity"`
}

type InventoryUsecase struct {
	repo InventoryRepo
}

func NewInventoryUsecase(repo InventoryRepo) *InventoryUsecase {
	return &InventoryUsecase{repo: repo}
}

func (uc *InventoryUsecase) CreateSKU(ctx context.Context, productID int64, skus []*Inventory) error {
	return uc.repo.CreateSKU(ctx, productID, skus)
}

func (uc *InventoryUsecase) UpdateSKU(ctx context.Context, skus []*Inventory) error {
	return uc.repo.UpdateSKU(ctx, skus)
}

func (uc *InventoryUsecase) ListSKUs(ctx context.Context, productID int64) ([]*Inventory, error) {
	return uc.repo.ListSKUs(ctx, productID)
}
