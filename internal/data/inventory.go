package data

import (
	"azushop/internal/biz"
	"context"
	"errors"

	"github.com/google/uuid"
)

type InventoryRepo struct {
	data *Data
}

func NewInventoryRepo(data *Data) biz.InventoryRepo {
	return &InventoryRepo{data: data}
}

func (r *InventoryRepo) UpdateInventories(ctx context.Context, inventories []*biz.Inventory, paths []string) error {
	gormClient := r.data.gormClient
	for _, inv := range inventories {
		vals := map[string]interface{}{}
		for _, path := range paths {
			switch path {
			case "stock_quantity":
				vals["stock_quantity"] = inv.StockQuantity
			case "reserved_quantity":
				vals["reserved_quantity"] = inv.ReservedQuantity
			}
		}
		if len(vals) == 0 {
			return errors.New("no valid paths")
		}
		if err := gormClient.WithContext(ctx).Model(inv).Updates(vals).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *InventoryRepo) BatchGetInventories(ctx context.Context, skuIDs []uuid.UUID) ([]*biz.Inventory, error) {
	if len(skuIDs) == 0 {
		return nil, errors.New("empty skuIDs")
	}
	gormClient := r.data.gormClient
	var inventories []*biz.Inventory
	err := gormClient.WithContext(ctx).Where("sku_id IN ?", skuIDs).Find(&inventories).Error
	if err != nil {
		return nil, err
	}
	return inventories, nil
}
