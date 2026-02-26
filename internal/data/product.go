package data

import (
	"azushop/internal/biz"
	"azushop/internal/common"
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

func cacheKeyProductSet(sellerID int32) string {
	return fmt.Sprintf("product:%d", sellerID)
}

func (repo *ProductRepo) ListProductsBySellerId(
	ctx context.Context,
	sellerID int32,
	pageToken int64,
	pageSize int32,
	productStatus biz.ProductStatus,
) ([]*biz.Product, error) {
	fullKey := cacheKeyProduct(sellerID, pageToken, pageSize, productStatus)
	if cachedProducts, found := GetCache[[]*biz.Product](ctx, repo.data, fullKey); found {
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

	SetCache(ctx, repo.data, fullKey, products, time.Minute)
	setKey := cacheKeyProductSet(sellerID)
	SetCacheSAdd(ctx, repo.data, setKey, fullKey)

	return products, nil
}

// TODO: use version.
func delProductCaches(ctx context.Context, data *Data, setKeys []string) {
	for _, setKey := range setKeys {
		mems, ok := GetCacheSMembers(ctx, data, setKey)
		if !ok {
			continue
		}
		DelCache(ctx, data, mems...)
		DelCache(ctx, data, setKey)
	}
}

// TODO: split it into different functions.
func (repo *ProductRepo) BatchUpsertProducts(ctx context.Context, products []*biz.Product, paths []string) error {
	if len(products) == 0 {
		return nil
	}

	client := repo.data.postgresClient
	ss := common.NewStringSet()

	if len(paths) == 0 {
		// insert
		colNames := []string{"product_name", "seller_id", "status"}
		rowValues := make([][]any, 0, len(products))
		for _, p := range products {
			ss.Insert(cacheKeyProductSet(p.SellerID))
			rowValues = append(rowValues, []any{p.ProductName, p.SellerID, p.ProductStatus})
		}
		stmt, values := sql.BuildBatchInsertSQL("products", colNames, rowValues)
		if _, err := client.ExecContext(ctx, stmt, values...); err != nil {
			return err
		}
		delProductCaches(ctx, repo.data, ss.ToSlice())
		return nil
	}
	// update
	lenPaths := len(paths)
	ids := make([]any, 0, len(products))
	colNames := make([]string, 0, lenPaths)
	colVals := make([][]any, 0, lenPaths)
	productNames := make([]any, 0, len(products))

	for _, product := range products {
		ids = append(ids, product.ID)
		ss.Insert(cacheKeyProductSet(product.SellerID))
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
	if _, err := client.ExecContext(ctx, stmt, values...); err != nil {
		return err
	}

	delProductCaches(ctx, repo.data, ss.ToSlice())
	return nil
}
