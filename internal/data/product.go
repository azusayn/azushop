package data

import (
	"azushop/internal/biz"
	"context"
)

type ProductRepo struct {
	data *Data
}

func NewProductRepo(data *Data) biz.ProductRepo {
	return &ProductRepo{data: data}
}

// TODO: cache and search
func (repo *ProductRepo) ListProducts(ctx context.Context, page, pageSize int64) ([]*biz.Product, error) {
	client := repo.data.postgresClient
	var products []*biz.Product
	stmt := "select id, product_name, seller_id from products limit $1 offset $2"
	rows, err := client.QueryContext(ctx, stmt, pageSize, page*pageSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var product biz.Product
		if err := rows.Scan(&product.ID, &product.ProductName, &product.SellerID); err != nil {
			return nil, err
		}
		stmt := "select id, attrs, stock_quantity, unit_price from skus where product_id=$1"
		rows, err := client.QueryContext(ctx, stmt, product.ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var sku biz.Sku
			if err := rows.Scan(&sku.ID, &sku.Attrs, &sku.StockQuantity, &sku.UnitPrice); err != nil {
				return nil, err
			}
			product.Skus = append(product.Skus, &sku)
		}
		rows.Close()
		products = append(products, &product)
	}
	return products, nil
}
