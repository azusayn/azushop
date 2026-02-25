package data

import (
	"azushop/internal/biz"
	"context"
	"fmt"
	"time"

	"github.com/azusayn/azutils/sql"
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

func (repo *ProductRepo) BatchUpsertProducts(ctx context.Context, products []*biz.Product, paths []string) error {
	lenProducts := len(products)
	if lenProducts == 0 {
		return nil
	}

	client := repo.data.postgresClient

	// insert
	if len(paths) == 0 {
		colNames := []string{"product_name", "seller_id", "status"}
		rowValues := [][]any{}
		for _, p := range products {
			rowValues = append(rowValues, []any{p.ProductName, p.SellerID, p.ProductStatus})
		}
		stmt, values := sql.BuildBatchInsertSQL("products", colNames, rowValues)
		_, err := client.ExecContext(ctx, stmt, values...)
		return err
	}

	// update
	// TODO: reflection lib for this.
	lenPaths := len(paths)
	ids := make([]any, 0, lenProducts)
	productNames := make([]any, 0, lenProducts)
	colNames := make([]string, 0, lenPaths)
	colVals := make([][]any, 0, lenPaths)
	for _, product := range products {
		ids = append(ids, product.ID)
		for _, path := range paths {
			switch path {
			case "product_name":
				productNames = append(productNames, product.ProductName)
			}
		}
	}
	if len(productNames) != 0 {
		colNames = append(colNames, "product_name")
		colVals = append(colVals, productNames)
	}
	stmt, values := sql.BuildBatchUpdateSQL("products", ids, colNames, colVals)
	_, err := client.ExecContext(ctx, stmt, values...)
	return err
}
