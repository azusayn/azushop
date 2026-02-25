package data

import (
	"azushop/internal/biz"
	"context"
	"fmt"
	"strings"
	"time"
)

type ProductRepo struct {
	data *Data
}

func NewProductRepo(data *Data) biz.ProductRepo {
	return &ProductRepo{data: data}
}

func cacheKeyProduct(sellerID int32, pageToken int64, pageSize int32, productStatus biz.ProductStatus) string {
	return fmt.Sprintf("product:%d:%d:%d:%s", sellerID, pageToken, pageSize, productStatus)
}

func (repo *ProductRepo) ListProductsBySellerId(
	ctx context.Context,
	sellerID int32,
	pageToken int64,
	pageSize int32,
	productStatus biz.ProductStatus,
) ([]*biz.Product, error) {
	key := cacheKeyProduct(sellerID, pageToken, pageSize, productStatus)
	if cachedProducts, found := GetCache[[]*biz.Product](ctx, repo.data, key); found {
		return cachedProducts, nil
	}

	client := repo.data.postgresClient

	stmt := `
		SELECT id, product_name, status
		FROM products
		WHERE id > $1 AND seller_id = $2 %s
		ORDER BY id LIMIT $3
	`
	args := []any{pageToken, sellerID, pageSize}
	if productStatus != biz.ProductStatusUnspecified {
		stmt = fmt.Sprintf(stmt, "AND status=$4")
		args = append(args, productStatus)
	} else {
		stmt = fmt.Sprintf(stmt, "")
	}

	rows, err := client.QueryContext(ctx, stmt, args...)
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

	SetCache(ctx, repo.data, key, products, time.Minute)

	return products, nil
}

// if any update fails, all changes are rolled back.
func (repo *ProductRepo) BatchUpsertProducts(ctx context.Context, products []*biz.Product, paths []string) error {
	client := repo.data.postgresClient

	// insert
	if len(paths) == 0 {
		stmt := `
			INSERT INTO products(product_name, seller_id, status)
			VALUES %d
		`
		var args []string
		var values []any
		for i, p := range products {
			base := i * 3
			args = append(args, fmt.Sprintf("($%d, $%d, $%d)", base+1, base+2, base+3))
			values = append(values, p.ProductName, p.SellerID, biz.ProductStatusOffline)
		}
		stmt = fmt.Sprintf(stmt, strings.Join(args, ","))
		_, err := client.ExecContext(ctx, stmt, values...)
		return err
	}

	// update
	// TODO: make it as an API in utils...
	stmt := `
		UPDATE products
		SET 
			%s
		FROM
			(VALUES %s) AS v(%s)
		WHERE
			products.id=v.id
	`
	var args []string
	var values []any
	lenPaths := len(paths)
	for i, product := range products {
		values = append(values, product.ID)
		for _, path := range paths {
			switch path {
			case "product_name":
				values = append(values, product.ProductName)
			default:
				return fmt.Errorf("unknown update path %q", path)
			}
		}
		base := i * (lenPaths + 1)
		var placeholders []string
		for j := 0; j < lenPaths; j++ {
			placeholders = append(placeholders, fmt.Sprintf("$%d", base+j+1))
		}
		args = append(args, fmt.Sprintf("(%s)", strings.Join(placeholders, ",")))
	}

	var columnNames []string
	for _, p := range paths {
		switch p {
		case "product_name":
			columnNames = append(columnNames, "product_name")
		}
	}
	var sets []string
	for _, c := range columnNames {
		sets = append(sets, fmt.Sprintf("%s=v.%s", c, c))
	}

	placeHolder1 := strings.Join(sets, ",")
	placeHolder2 := strings.Join(args, ",")
	placeHolder3 := strings.Join(columnNames, ",")
	stmt = fmt.Sprintf(stmt, placeHolder1, placeHolder2, placeHolder3)

	_, err := client.ExecContext(ctx, stmt, values...)
	return err
}
