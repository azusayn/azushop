package biz

import (
	"context"
	"fmt"

	"github.com/azusayn/azutils/auth"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type ProductStatus string

const (
	ProductStatusUnspecified ProductStatus = "unspecified"
	ProductStatusDraft       ProductStatus = "draft"
	ProductStatusPending     ProductStatus = "pending"
	ProductStatusActive      ProductStatus = "active"
	ProductStatusOffline     ProductStatus = "offline"
)

type Product struct {
	ID            int64
	ProductName   string
	SellerID      int32
	ProductStatus ProductStatus
}

type ProductRepo interface {
	ListProductsBySellerId(ctx context.Context, sellerID int32, pageToken int64, pageSize int32, productStatus ProductStatus) ([]*Product, error)
	BatchUpsertProducts(ctx context.Context, product []*Product, paths []string) error
}

type ProductUsecase struct {
	repo ProductRepo
}

func NewProductUsecase(repo ProductRepo) *ProductUsecase {
	return &ProductUsecase{
		repo: repo,
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

// userID is the user calling the API.
func (uc *ProductUsecase) ListSellerProducts(
	ctx context.Context,
	sellerID int32,
	pageToken int64,
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
		return products, nil
	case UserRoleMerchant:
		nc := auth.NewNameChecker(
			auth.WithLengthLimit(1, 20),
			auth.WithAllowSpace(),
			auth.WithAllowPunct(),
		)
		for _, p := range products {
			p.SellerID = userID
			if err := nc.BasicCheck(p.ProductName); err != nil {
				return nil, err
			}
			p.ProductStatus = ProductStatusOffline
		}

		return products, nil
	default:
	}
	return nil, fmt.Errorf("role %q doesn't have the permission to upsert products", role)
}

// TODO: move it to proper place.
func uniqueStrings(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := paths[:0]
	for _, p := range paths {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			result = append(result, p)
		}
	}
	return result
}

func (uc *ProductUsecase) BatchUpsertProducts(
	ctx context.Context,
	products []*Product,
	updateMask *fieldmaskpb.FieldMask,
	userID int32,
	userRole UserRole,
) error {
	products, err := productsFilter(products, userID, userRole)
	if err != nil {
		return err
	}
	return uc.repo.BatchUpsertProducts(ctx, products, uniqueStrings(updateMask.Paths))
}
