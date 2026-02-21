package biz

import (
	pb "azushop/api/product/v1"
	"context"
	"encoding/json"

	"google.golang.org/protobuf/types/known/structpb"
)

type Sku struct {
	ID            int64
	ProductID     int64
	Attrs         json.RawMessage
	StockQuantity int64
	UnitPrice     string
}

type Product struct {
	ID          int64
	ProductName string
	// TODO: just extract seller_id from auth token.
	SellerID int32
	Skus     []*Sku
}

type ProductRepo interface {
	ListProducts(ctx context.Context, page, pageSize int64) ([]*Product, error)
}

type ProductUsecase struct {
	repo ProductRepo
}

func NewProductUsecase(repo ProductRepo) *ProductUsecase {
	return &ProductUsecase{
		repo: repo,
	}
}

func (uc *ProductUsecase) ListProducts(ctx context.Context, page, pageSize int64) ([]*pb.Product, error) {
	products, err := uc.repo.ListProducts(ctx, page, pageSize)
	if err != nil {
		return nil, err
	}
	var pbProducts []*pb.Product
	for _, p := range products {
		var pbSkus []*pb.Sku
		for _, sku := range p.Skus {
			var s structpb.Struct
			if err := s.UnmarshalJSON(sku.Attrs); err != nil {
				return nil, err
			}
			pbSkus = append(pbSkus, &pb.Sku{
				Id:            sku.ID,
				ProductId:     sku.ProductID,
				Attrs:         &s,
				StockQuantity: sku.StockQuantity,
				UnitPrice:     sku.UnitPrice,
			})
		}
		pbProducts = append(pbProducts, &pb.Product{
			Id:          p.ID,
			ProductName: p.ProductName,
			SellerId:    p.SellerID,
			Skus:        pbSkus,
		})
	}
	return pbProducts, nil
}
