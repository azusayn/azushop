package data

import (
	"azushop/internal/biz"
	"context"
	"encoding/json"
)

type ProductRepo struct {
	data *Data
}

func NewProductRepo(data *Data) biz.ProductRepo {
	return &ProductRepo{data: data}
}

// TODO: cache and search
func (repo *ProductRepo) ListProducts(ctx context.Context, pageToken int64, pageSize int32) ([]*biz.Product, error) {
	client := repo.data.postgresClient

	// product id -> []Sku
	skusMap := make(map[int64][]*biz.Sku)
	productMap := make(map[int64]*biz.Product)
	stmt := `
		SELECT p.id, p.product_name, p.seller_id, 
		s.id, s.attrs, s.unit_price, s.stock_quantity 
		FROM (
			SELECT id, product_name, seller_id 
			FROM products 
			WHERE id > $1 
			ORDER BY id 
			LIMIT $2
		) p 
		LEFT JOIN skus s ON p.id=s.product_id`
	rows, err := client.QueryContext(ctx, stmt, pageToken, pageSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			productID     int64
			productName   string
			sellerID      int32
			skuID         int64
			attrs         json.RawMessage
			unitPrice     string
			stockQuantity int64
		)
		if err := rows.Scan(&productID, &productName, &sellerID, &skuID, &attrs, &unitPrice, &stockQuantity); err != nil {
			return nil, err
		}
		sku := &biz.Sku{
			ID:            skuID,
			Attrs:         attrs,
			UnitPrice:     unitPrice,
			StockQuantity: stockQuantity,
		}
		if skus, ok := skusMap[productID]; ok {
			skusMap[productID] = append(skus, sku)
			continue
		}
		productMap[productID] = &biz.Product{
			ID:          productID,
			ProductName: productName,
			SellerID:    sellerID,
		}
		skusMap[productID] = []*biz.Sku{sku}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var products []*biz.Product
	for productID, product := range productMap {
		product.Skus = skusMap[productID]
		products = append(products, product)
	}
	return products, nil
}
