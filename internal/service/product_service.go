package service

import (
	pb "azushop/api/product/v1"
	"azushop/internal/biz"
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func (s *ProductService) ListProducts(ctx context.Context, req *pb.ListProductsRequest) (*pb.ListProductsResponse, error) {
	if req.PageSize > maxPageSize {
		return nil, status.Error(codes.OutOfRange, codes.OutOfRange.String())
	}
	products, err := s.uc.ListProducts(ctx, req.Page, req.PageSize)
	if err != nil {
		return nil, err
	}
	return &pb.ListProductsResponse{
		Products: products,
	}, nil
}
