package service

import (
	pb "azushop/api/product/v1"
	"azushop/internal/biz"
	"context"
	"encoding/json"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type ProductService struct {
	pb.UnimplementedProductServiceServer
	uc *biz.ProductUsecase
}

func NewProductService(uc *biz.ProductUsecase) *ProductService {
	return &ProductService{uc: uc}
}

const (
	maxPageSize = 100
)

func convertToPbProduct(product *biz.Product) *pb.Product {
	var pbSkus []*pb.Sku
	for _, sku := range product.Skus {
		var s structpb.Struct
		if err := s.UnmarshalJSON(sku.Attrs); err != nil {
			continue
		}
		pbSkus = append(pbSkus, &pb.Sku{
			Id:            sku.ID,
			ProductId:     sku.ProductID,
			Attrs:         &s,
			StockQuantity: sku.StockQuantity,
			UnitPrice:     sku.UnitPrice,
		})
	}
	return &pb.Product{
		Id:          product.ID,
		ProductName: product.ProductName,
		SellerId:    product.SellerID,
		Skus:        pbSkus,
	}
}

func convertToBizProduct(pbProduct *pb.Product) *biz.Product {
	var skus []*biz.Sku
	for _, s := range pbProduct.Skus {
		skus = append(skus, &biz.Sku{
			Attrs:         json.RawMessage(s.String()),
			StockQuantity: s.StockQuantity,
			UnitPrice:     s.UnitPrice,
		})
	}
	return &biz.Product{
		ID:          pbProduct.Id,
		ProductName: pbProduct.ProductName,
		SellerID:    pbProduct.SellerId,
		Skus:        skus,
	}
}

func (s *ProductService) ListProducts(ctx context.Context, req *pb.ListProductsRequest) (*pb.ListProductsResponse, error) {
	if req.PageSize > maxPageSize {
		return nil, status.Error(codes.OutOfRange, codes.OutOfRange.String())
	}
	products, err := s.uc.ListProducts(ctx, req.PageToken, req.PageSize)
	if err != nil {
		return nil, err
	}
	return &pb.ListProductsResponse{
		Products: products,
	}, nil
}

func (s *ProductService) UpsertProduct(ctx context.Context, req *pb.UpsertProductRequest) error {
	if len(req.Skus) <= 0 {
		return status.Error(codes.InvalidArgument, codes.InvalidArgument.String())
	}
	product := &pb.Product{
		Id:          req.Id,
		ProductName: req.ProductName,
		SellerId:    req.SellerId,
		Skus:        req.Skus,
	}
	return s.uc.UpsertProduct(ctx, convertToBizProduct(product), req.UpdateMask)
}
