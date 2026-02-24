package data

import (
	"azushop/internal/biz"
	"context"
	"fmt"
	"time"
)

type ProductRepo struct {
	data *Data
}

func NewProductRepo(data *Data) biz.ProductRepo {
	return &ProductRepo{data: data}
}

func cacheKeyProduct(pageToken int64, pageSize int32) string {
	return fmt.Sprintf("product:%d:%d", pageToken, pageSize)
}

func (repo *ProductRepo) ListProductsBySellerId(
	ctx context.Context,
	sellerID int32,
	pageToken int64,
	pageSize int32,
	productStatus biz.ProductStatus,
) ([]*biz.Product, error) {
	key := cacheKeyProduct(pageToken, pageSize)
	cachedProducts, found := GetCache[[]*biz.Product](ctx, repo.data, key)
	if found {
		return cachedProducts, nil
	}

	client := repo.data.postgresClient
	stmt := `
		SELECT id, product_name, product_status
		FROM products 
		WHERE id > $1 AND seller_id=$2 %s
		ORDER BY id LIMIT $3
	`
	if productStatus != biz.ProductStatusUnspecified {
		stmt = fmt.Sprintf(stmt, "AND status="+string(productStatus))
	} else {
		stmt = fmt.Sprintf(stmt, "")
	}

	rows, err := client.QueryContext(ctx, stmt, pageToken, sellerID, pageSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []*biz.Product
	for rows.Next() {
		var product biz.Product
		if err := rows.Scan(&product.ID, &product.ProductName, &product.ProductStatus); err != nil {
			return nil, err
		}
		products = append(products, &product)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	SetCache(ctx, repo.data, key, products, time.Duration(0))

	return products, nil
}

func (repo *ProductRepo) UpsertProduct(ctx context.Context, product *biz.Product, paths []string) error {
	panic("unimplemented")
}

// func (repo *ProductRepo) UpsertProduct(ctx context.Context, product *biz.Product, paths []string) error {
// 	client := repo.data.postgresClient
// 	tx, err := client.BeginTx(ctx, nil)
// 	if err != nil {
// 		return err
// 	}
// 	defer tx.Rollback()

// 	// insert
// 	if len(paths) == 0 {
// 		stmt := `
// 			INSERT INTO products(product_name, seller_id)
// 			VALUES ($1, $2)
// 			RETURNING id
// 		`
// 		var productID int64
// 		if err := tx.QueryRowContext(ctx, stmt, product.ProductName, product.SellerID).Scan(&productID); err != nil {
// 			return err
// 		}
// 		stmt = `
// 			INSERT INTO skus(product_id, attrs, stock_quantity, unit_price)
// 			VALUES
// 		`
// 		var values []any
// 		var indice []string
// 		for i, sku := range product.Skus {
// 			base := 4 * i
// 			indice = append(indice, fmt.Sprintf("($%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4))
// 			values = append(values, productID, sku.Attrs, sku.StockQuantity, sku.UnitPrice)
// 		}
// 		stmt = stmt + strings.Join(indice, ",")
// 		if _, err := tx.ExecContext(ctx, stmt, values...); err != nil {
// 			return err
// 		}
// 	} else {
// 		// update
// 		for _, path := range paths {
// 			switch path {
// 			case "product_name":
// 				stmt := `
// 					UPDATE products
// 					SET product_name=$1
// 					WHERE id=$2
// 				`
// 				if _, err := tx.ExecContext(ctx, stmt, product.ProductName, product.ID); err != nil {
// 					return err
// 				}
// 			case "skus":
// 				var indice []string
// 				var values []any
// 				for i, sku := range product.Skus {
// 					base := 4 * i
// 					indice = append(indice, fmt.Sprintf("($%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4))
// 					values = append(values, sku.ID, sku.Attrs, sku.StockQuantity, sku.UnitPrice)
// 				}
// 				stmt := `
// 					UPDATE skus
// 					SET
// 						attrs=v.attrs,
// 						stock_quantity=v.stock_quantity,
// 						unit_price=v.unit_price
// 					FROM
// 						(VALUES %s) AS v(id, attrs, stock_quantity, unit_price)
// 					WHERE skus.id=v.id
// 				`
// 				stmt = fmt.Sprintf(stmt, strings.Join(indice, ","))
// 				if _, err := tx.ExecContext(ctx, stmt, values...); err != nil {
// 					return err
// 				}
// 			}
// 		}
// 	}

// 	return tx.Commit()
// }
