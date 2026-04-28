package data

import (
	"azushop/internal/biz"
	"azushop/internal/common"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/azusayn/azutils/sql"
	"github.com/google/uuid"
	"github.com/google/wire"
)

var ProductDataProviderSet = wire.NewSet(
	NewPostgres,
	NewRedis,
	NewProductRepo,
	NewKafkaProducer,
	NewProductPublisher,
)

type ProductCreatedMessage struct {
	SkuIDs []uuid.UUID
}

const (
	productCacheTime = time.Minute
)

type ProductRepo struct {
	postgres *Postgres
	redis    *Redis
}

func NewProductRepo(postgres *Postgres, redis *Redis) biz.ProductRepo {
	return &ProductRepo{
		postgres: postgres,
		redis:    redis,
	}
}

func cacheKeyProduct(sellerID int32, pageToken uuid.UUID, pageSize int32, productStatus biz.ProductStatus) string {
	return fmt.Sprintf("product:%d:%s:%d:%s", sellerID, pageToken.String(), pageSize, productStatus)
}

func cacheKeyProductSet(sellerID int32) string {
	return fmt.Sprintf("product:%d", sellerID)
}

func (repo *ProductRepo) ListProductsBySellerId(
	ctx context.Context,
	sellerID int32,
	pageToken uuid.UUID,
	pageSize int32,
	productStatus biz.ProductStatus,
) ([]*biz.Product, error) {
	fullKey := cacheKeyProduct(sellerID, pageToken, pageSize, productStatus)
	if cachedProducts, found := GetCache[[]*biz.Product](ctx, repo.redis, fullKey); found {
		return cachedProducts, nil
	}

	client := repo.postgres.Conn

	stmt := `
		SELECT p.id, p.product_name, p.status, 
			s.id, s.attrs, s.unit_price
		FROM products p 
		JOIN skus s ON p.id=s.product_id
		WHERE p.id > $1 AND p.seller_id = $2 %s
		ORDER BY p.id LIMIT $3
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

	// mapping frop product ID to its SKUs.
	m := make(map[uuid.UUID][]*biz.Sku)
	var products []*biz.Product
	for rows.Next() {
		var (
			productID     uuid.UUID
			productName   string
			productStatus string
			skuID         uuid.UUID
			attrs         json.RawMessage
			unitPrice     string
		)
		if err := rows.Scan(
			&productID,
			&productName,
			&productStatus,
			&skuID,
			&attrs,
			&unitPrice,
		); err != nil {
			return nil, err
		}
		sku := &biz.Sku{
			ID:        skuID,
			Attrs:     attrs,
			UnitPrice: biz.Numeric(unitPrice),
			ProductID: productID,
		}
		if skus, ok := m[productID]; ok {
			m[productID] = append(skus, sku)
			continue
		}
		products = append(products, &biz.Product{
			ID:            productID,
			ProductName:   productName,
			SellerID:      sellerID,
			ProductStatus: biz.ProductStatus(productStatus),
		})
		m[productID] = []*biz.Sku{sku}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, p := range products {
		p.Skus = m[p.ID]
	}

	SetCache(ctx, repo.redis, fullKey, products, productCacheTime)
	setKey := cacheKeyProductSet(sellerID)
	SetCacheSAdd(ctx, repo.redis, setKey, fullKey)

	return products, nil
}

// TODO(2): use version.
func delProductCaches(ctx context.Context, r *Redis, setKeys []string) {
	for _, setKey := range setKeys {
		mems, ok := GetCacheSMembers(ctx, r, setKey)
		if !ok {
			continue
		}
		DelCache(ctx, r, mems...)
		DelCache(ctx, r, setKey)
	}
}

func (repo *ProductRepo) BatchCreateProducts(ctx context.Context, products []*biz.Product) ([]*biz.Product, error) {
	client := repo.postgres.Conn

	ss := common.NewStringSet()
	productsColNames := []string{"id", "product_name", "seller_id", "status"}
	productsRowValues := make([][]any, 0, len(products))
	skusColNames := []string{"id", "product_id", "attrs", "unit_price"}
	skusRowValues := make([][]any, 0)
	for _, p := range products {
		ss.Insert(cacheKeyProductSet(p.SellerID))
		productID, err := uuid.NewV7()
		if err != nil {
			return nil, err
		}
		p.ID = productID
		productsRowValues = append(productsRowValues, []any{productID, p.ProductName, p.SellerID, p.ProductStatus})
		for _, s := range p.Skus {
			skuID, err := uuid.NewV7()
			if err != nil {
				return nil, err
			}
			s.ID = skuID
			skusRowValues = append(skusRowValues, []any{s.ID, p.ID, s.Attrs, s.UnitPrice})
		}
	}

	tx, err := client.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	stmtP, valuesP := sql.BuildBatchInsertSQL("products", productsColNames, productsRowValues)
	if _, err := tx.ExecContext(ctx, stmtP, valuesP...); err != nil {
		return nil, err
	}
	stmtS, valuesS := sql.BuildBatchInsertSQL("skus", skusColNames, skusRowValues)
	if _, err := tx.ExecContext(ctx, stmtS, valuesS...); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	delProductCaches(ctx, repo.redis, ss.ToSlice())
	return products, nil
}

func (repo *ProductRepo) BatchUpdateProducts(ctx context.Context, products []*biz.Product, paths []string) error {
	if len(products) == 0 || len(paths) == 0 {
		return nil
	}

	client := repo.postgres.Conn
	ss := common.NewStringSet()

	lenPaths := len(paths)
	productIds := make([]any, 0, len(products))
	productNames := make([]any, 0, len(products))
	skuIDs := make([]any, 0)
	attrs := make([]any, 0)
	unitPrices := make([]any, 0)

	for _, product := range products {
		productIds = append(productIds, product.ID)
		ss.Insert(cacheKeyProductSet(product.SellerID))
		for _, path := range paths {
			switch path {
			case "product_name":
				productNames = append(productNames, product.ProductName)
			case "skus":
				if len(product.Skus) == 0 {
					return errors.New("empty skus")
				}
				for _, pSku := range product.Skus {
					skuIDs = append(skuIDs, pSku.ID)
					attrs = append(attrs, pSku.Attrs)
					unitPrices = append(unitPrices, pSku.UnitPrice)
				}
			}
		}
	}

	tx, err := client.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// table 'products'
	productColNames := make([]string, 0, lenPaths+1)
	productColVals := make([][]any, 0, lenPaths+1)
	productColTypes := []string{"::UUID"}
	productColNames = append(productColNames, "id")
	productColVals = append(productColVals, productIds)
	if len(productNames) != 0 {
		productColNames = append(productColNames, "product_name")
		productColVals = append(productColVals, productNames)
		productColTypes = append(productColTypes, "")
	}
	if len(productColNames) != 0 {
		stmt, values := sql.BuildBatchUpdateSQL("products", productColNames, productColTypes, productColVals)
		if _, err := tx.ExecContext(ctx, stmt, values...); err != nil {
			return err
		}
	}

	// table skus
	skuColNames := []string{"id"}
	skuColTypes := []string{"::UUID"}
	skuColVals := [][]any{skuIDs}
	if len(attrs) != 0 && len(unitPrices) != 0 {
		skuColNames = append(skuColNames, "attrs", "unit_price")
		skuColVals = append(skuColVals, attrs, unitPrices)
		skuColTypes = append(skuColTypes, "::JSON", "::NUMERIC(10,2)")
	}
	if len(skuColNames) != 0 {
		stmt, values := sql.BuildBatchUpdateSQL("skus", skuColNames, skuColTypes, skuColVals)
		if _, err := tx.ExecContext(ctx, stmt, values...); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	delProductCaches(ctx, repo.redis, ss.ToSlice())
	return nil
}

func (repo *ProductRepo) BatchGetSkuDetails(
	ctx context.Context,
	skuIDs []uuid.UUID,
	pageToken uuid.UUID,
	pageSize int32,
) ([]*biz.SkuDetail, error) {
	client := repo.postgres.Conn
	stmt := `
		SELECT 
			p.product_name, 
			s.id, 
			s.product_id, 
			s.attrs, 
			s.unit_price
		FROM skus s
		JOIN products p ON p.id = s.product_id
		WHERE s.id IN (%s) AND s.id > $1
		ORDER BY s.id
		LIMIT $2
	`
	var args []string
	values := []any{pageToken, pageSize}
	for i, skuID := range skuIDs {
		args = append(args, fmt.Sprintf("$%d", i+3))
		values = append(values, skuID)
	}
	stmt = fmt.Sprintf(stmt, strings.Join(args, ","))
	rows, err := client.QueryContext(ctx, stmt, values...)
	if err != nil {
		return nil, err
	}
	var skus []*biz.SkuDetail
	for rows.Next() {
		var skuDetail biz.SkuDetail
		if err := rows.Scan(
			&skuDetail.ProductName,
			&skuDetail.Sku.ID,
			&skuDetail.Sku.ProductID,
			&skuDetail.Sku.Attrs,
			&skuDetail.Sku.UnitPrice,
		); err != nil {
			return nil, err
		}
		skus = append(skus, &skuDetail)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return skus, nil
}

type ProductPublisher struct {
	kafkaProducer *KafkaProducer
}

func NewProductPublisher(producer *KafkaProducer) biz.ProductPublisher {
	return &ProductPublisher{kafkaProducer: producer}
}

func (p *ProductPublisher) PublishProductCreated(ctx context.Context, skuIDs []uuid.UUID) error {
	prodcuer := p.kafkaProducer.syncProducer
	if prodcuer == nil {
		return errors.New("nil producer")
	}
	msg := ProductCreatedMessage{
		SkuIDs: skuIDs,
	}
	bytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	prodMsg := &sarama.ProducerMessage{
		Topic: biz.KafkaTopicProductCreated,
		Value: sarama.ByteEncoder(bytes),
	}
	_, _, err = prodcuer.SendMessage(prodMsg)
	return err
}
