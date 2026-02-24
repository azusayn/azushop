package biz

import (
	"context"

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
	UpsertProduct(ctx context.Context, product *Product, paths []string) error
}

type ProductUsecase struct {
	repo ProductRepo
}

func NewProductUsecase(repo ProductRepo) *ProductUsecase {
	return &ProductUsecase{
		repo: repo,
	}
}

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

func (uc *ProductUsecase) UpsertProduct(ctx context.Context, product *Product, updateMask *fieldmaskpb.FieldMask) error {
	err := uc.repo.UpsertProduct(ctx, product, updateMask.Paths)
	if err != nil {
		return err
	}
	return nil
}
