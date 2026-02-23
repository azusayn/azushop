package biz

import (
	"context"

	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type Product struct {
	ID          int64
	ProductName string
	// TODO: just extract seller_id from auth token.
	SellerID int32
	Skus     []*Sku
}

type ProductRepo interface {
	ListProductsBySellerId(ctx context.Context, sellerID int32, pageToken int64, pageSize int32) ([]*Product, error)
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

func (uc *ProductUsecase) ListProductsBySellerId(ctx context.Context, sellerID int32, pageToken int64, pageSize int32) ([]*Product, error) {
	products, err := uc.repo.ListProductsBySellerId(ctx, sellerID, pageToken, pageSize)
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
