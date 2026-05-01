package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/azusayn/azutils/validate"
	"github.com/google/uuid"
)

type ProductStatus string

const (
	ProductStatusUnspecified ProductStatus = "unspecified"
	ProductStatusDraft       ProductStatus = "draft"
	ProductStatusPending     ProductStatus = "pending"
	ProductStatusActive      ProductStatus = "active"
	ProductStatusOffline     ProductStatus = "offline"
)

type Numeric string

type Sku struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	// map[string]string
	Attrs     json.RawMessage
	UnitPrice Numeric
}

type SkuDetail struct {
	Sku         Sku
	ProductName string
}

type Product struct {
	ID            uuid.UUID
	ProductName   string
	SellerID      int32
	ProductStatus ProductStatus
	Skus          []*Sku
}

type ProductRepo interface {
	ListProductsBySellerId(ctx context.Context, sellerID int32, pageToken uuid.UUID, pageSize int32, productStatus ProductStatus) ([]*Product, error)
	BatchCreateProducts(ctx context.Context, product []*Product) ([]*Product, error)
	BatchUpdateProducts(ctx context.Context, product []*Product, paths []string) error
	BatchGetSkuDetails(ctx context.Context, skuIDs []uuid.UUID, pageToken uuid.UUID, pageSize int32) ([]*SkuDetail, error)
}

type ProductPublisher interface {
	PublishProductCreated(ctx context.Context, skuIDs []uuid.UUID) error
}

type ProductUsecase struct {
	repo      ProductRepo
	publisher ProductPublisher
}

func NewProductUsecase(repo ProductRepo, publisher ProductPublisher) *ProductUsecase {
	return &ProductUsecase{
		repo:      repo,
		publisher: publisher,
	}
}

// used by listSellerProducts()
func productStatusFilter(productStatus ProductStatus, sellerID, userID int32, role UserRole) ProductStatus {
	switch role {
	case UserRoleAdministrator:
		return productStatus

	case UserRoleMerchant:
		if sellerID == userID {
			return productStatus
		}
		return ProductStatusActive

	case UserRoleCustomer:
		return ProductStatusActive

	default:
	}
	return ProductStatusUnspecified
}

// TODO(3): product quantity limits.
func (uc *ProductUsecase) BatchCheckProducts(products []*Product) error {
	nc := validate.NewNameChecker(
		validate.WithLengthLimit(1, 50),
		validate.WithAllowSpace(),
		validate.WithAllowPunct(),
	)
	if len(products) == 0 {
		return errors.New("empty products")
	}
	for _, p := range products {
		if err := nc.BasicCheck(p.ProductName); err != nil {
			return err
		}
		if len(p.Skus) == 0 {
			return fmt.Errorf("empty skus for product %q", p.ProductName)
		}
	}
	return nil
}

// userID is the user calling the API.
func (uc *ProductUsecase) ListSellerProducts(
	ctx context.Context,
	sellerID int32,
	pageToken uuid.UUID,
	pageSize int32,
	productStatus ProductStatus,
	userID int32,
	userRole UserRole,
) ([]*Product, error) {
	productStatus = productStatusFilter(productStatus, sellerID, userID, userRole)
	products, err := uc.repo.ListProductsBySellerId(ctx, sellerID, pageToken, pageSize, productStatus)
	if err != nil {
		return nil, err
	}
	return products, nil
}

func productsFilter(products []*Product, userID int32, role UserRole) ([]*Product, error) {
	switch role {
	case UserRoleAdministrator:
		// TODO(3): pass seller ID in...
		return products, nil
	case UserRoleMerchant:
		for _, p := range products {
			p.SellerID = userID
			p.ProductStatus = ProductStatusOffline
		}

		return products, nil
	default:
	}
	return nil, fmt.Errorf("role %q doesn't have the permission to upsert products", role)
}

func (uc *ProductUsecase) BatchCreateProducts(
	ctx context.Context,
	products []*Product,
	userID int32,
	userRole UserRole,
) ([]*Product, error) {
	products, err := productsFilter(products, userID, userRole)
	if err != nil {
		return nil, err
	}
	createdProds, err := uc.repo.BatchCreateProducts(ctx, products)
	if err != nil {
		return nil, err
	}

	var skuIDs []uuid.UUID
	for _, p := range createdProds {
		for _, s := range p.Skus {
			skuIDs = append(skuIDs, s.ID)
		}
	}
	// TODO(0): outbox
	if err := uc.publisher.PublishProductCreated(ctx, skuIDs); err != nil {
		slog.Warn(err.Error())
	}
	return createdProds, nil
}

func (uc *ProductUsecase) BatchUpdateProducts(
	ctx context.Context,
	products []*Product,
	paths []string,
	userID int32,
	userRole UserRole,
) error {
	products, err := productsFilter(products, userID, userRole)
	if err != nil {
		return err
	}
	return uc.repo.BatchUpdateProducts(ctx, products, paths)
}

func (uc *ProductUsecase) BatchGetSkuDetails(ctx context.Context, skuIDs []uuid.UUID, pageToken uuid.UUID, pageSize int32) ([]*SkuDetail, error) {
	return uc.repo.BatchGetSkuDetails(ctx, skuIDs, pageToken, pageSize)
}
