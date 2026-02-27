package biz

import (
	"context"

	"github.com/google/uuid"
)

type InventoryRepo interface {
	UpdateInventories(ctx context.Context, inventories []*Inventory, paths []string) error
	BatchGetInventories(ctx context.Context, skuIDs []uuid.UUID) ([]*Inventory, error)
}

type Inventory struct {
	ID               int64 `gorm:"column:sku_id"`
	StockQuantity    int64 `gorm:"column:stock_quantity"`
	ReservedQuantity int64 `gorm:"column:reserved_quantity"`
}

type InventoryUsecase struct {
	repo InventoryRepo
}

func NewInventoryUsecase(repo InventoryRepo) *InventoryUsecase {
	return &InventoryUsecase{repo: repo}
}
