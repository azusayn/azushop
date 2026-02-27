package data

import (
	"azushop/internal/biz"
	"context"
)

type InventoryRepo struct {
	data *Data
}

func NewInventoryRepo(data *Data) biz.InventoryRepo {
	return &InventoryRepo{data: data}
}
func (r *InventoryRepo) CreateSKU(ctx context.Context, productID int64, skus []*biz.Inventory) error {
	gormClient := r.data.gormClient
	for _, sku := range skus {
		sku.ProductID = productID
	}
	return gormClient.WithContext(ctx).Create(skus).Error
}

func (r *InventoryRepo) UpdateSKU(ctx context.Context, skus []*biz.Inventory) error {
	gormClient := r.data.gormClient
	for _, sku := range skus {
		vals := map[string]interface{}{
			"unit_price": sku.UnitPrice,
			"attrs":      sku.Attrs,
		}
		if err := gormClient.WithContext(ctx).Model(sku).Updates(vals).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *InventoryRepo) ListSKUs(ctx context.Context, productID int64) ([]*biz.Inventory, error) {
	gormClient := r.data.gormClient
	var skus []*biz.Inventory
	err := gormClient.WithContext(ctx).Where("product_id = ?", productID).Find(&skus).Error
	if err != nil {
		return nil, err
	}
	return skus, nil
}
