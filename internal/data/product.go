package data

import (
	"azushop/internal/biz"
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
		LEFT JOIN skus s ON p.id=s.product_id
	`
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

func (repo *ProductRepo) UpsertProduct(ctx context.Context, product *biz.Product, paths []string) error {
	client := repo.data.postgresClient
	tx, err := client.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// insert
	if len(paths) == 0 {
		stmt := `
			INSERT INTO products(product_name, seller_id)
			VALUES ($1, $2)
			RETURNING id
		`
		var productID int64
		if err := tx.QueryRowContext(ctx, stmt, product.ProductName, product.SellerID).Scan(&productID); err != nil {
			return err
		}
		stmt = `
			INSERT INTO skus(product_id, attrs, stock_quantity, unit_price) 
			VALUES 
		`
		var values []any
		var indice []string
		for i, sku := range product.Skus {
			base := 4 * i
			indice = append(indice, fmt.Sprintf("($%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4))
			values = append(values, productID, sku.Attrs, sku.StockQuantity, sku.UnitPrice)
		}
		stmt = stmt + strings.Join(indice, ",")
		if _, err := tx.ExecContext(ctx, stmt, values...); err != nil {
			return err
		}
	} else {
		// update
		for _, path := range paths {
			switch path {
			case "product_name":
				stmt := `
					UPDATE products
					SET product_name=$1
					WHERE id=$2
				`
				if _, err := tx.ExecContext(ctx, stmt, product.ProductName, product.ID); err != nil {
					return err
				}
			case "skus":
				var indice []string
				var values []any
				for i, sku := range product.Skus {
					base := 4 * i
					indice = append(indice, fmt.Sprintf("($%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4))
					values = append(values, sku.ID, sku.Attrs, sku.StockQuantity, sku.UnitPrice)
				}
				stmt := `
					UPDATE skus
					SET
						attrs=v.attrs,
						stock_quantity=v.stock_quantity,
						unit_price=v.unit_price
					FROM 
						(VALUES %s) AS v(id, attrs, stock_quantity, unit_price)
					WHERE skus.id=v.id
				`
				stmt = fmt.Sprintf(stmt, strings.Join(indice, ","))
				if _, err := tx.ExecContext(ctx, stmt, values...); err != nil {
					return err
				}
			}
		}
	}

	return tx.Commit()
}
