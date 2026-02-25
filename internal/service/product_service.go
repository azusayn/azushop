package service

import (
	pb "azushop/api/product/v1"
	"azushop/internal/biz"
	"azushop/internal/pkg/middleware"
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

func convertToPbProductStatus(productStatus *biz.ProductStatus) pb.ProductStatus {
	if productStatus != nil {
		switch *productStatus {
		case biz.ProductStatusDraft:
			return pb.ProductStatus_PRODUCT_STATUS_DRAFT
		case biz.ProductStatusPending:
			return pb.ProductStatus_PRODUCT_STATUS_PENDING
		case biz.ProductStatusActive:
			return pb.ProductStatus_PRODUCT_STATUS_ACTIVE
		case biz.ProductStatusOffline:
			return pb.ProductStatus_PRODUCT_STATUS_OFFLINE
		default:
		}
	}
	return pb.ProductStatus_PRODUCT_STATUS_UNSPECIFIED
}

func convertToBizProductStatus(productStatus *pb.ProductStatus) biz.ProductStatus {
	if productStatus != nil {
		switch *productStatus {
		case pb.ProductStatus_PRODUCT_STATUS_DRAFT:
			return biz.ProductStatusDraft
		case pb.ProductStatus_PRODUCT_STATUS_PENDING:
			return biz.ProductStatusPending
		case pb.ProductStatus_PRODUCT_STATUS_ACTIVE:
			return biz.ProductStatusActive
		case pb.ProductStatus_PRODUCT_STATUS_OFFLINE:
			return biz.ProductStatusOffline
		default:
		}
	}
	return biz.ProductStatusUnspecified
}

func convertToPbProducts(products []*biz.Product) []*pb.Product {
	var pbProducts []*pb.Product
	for _, p := range products {
		pbProducts = append(pbProducts, &pb.Product{
			Id:            p.ID,
			ProductName:   p.ProductName,
			SellerId:      p.SellerID,
			ProductStatus: convertToPbProductStatus(&p.ProductStatus),
		})
	}
	return pbProducts
}

func convertToBizProducts(products []*pb.Product) []*biz.Product {
	var bizProducts []*biz.Product
	for _, p := range products {
		bizProducts = append(bizProducts, &biz.Product{
			ID:            p.Id,
			ProductName:   p.ProductName,
			SellerID:      p.SellerId,
			ProductStatus: convertToBizProductStatus(&p.ProductStatus),
		})
	}
	return bizProducts
}

func (s *ProductService) ListSellerProducts(ctx context.Context, req *pb.ListSellerProductsRequest) (*pb.ListSellerProductsResponse, error) {
	if req.PageSize > maxPageSize {
		return nil, status.Error(codes.OutOfRange, codes.OutOfRange.String())
	}
	userID, role, err := middleware.ExtractUserInfo(&ctx)
	if err != nil {
		return nil, err
	}
	products, err := s.uc.ListSellerProducts(
		ctx,
		req.SellerId,
		req.PageToken,
		req.PageSize,
		convertToBizProductStatus(req.ProductStatus),
		userID,
		biz.UserRole(role),
	)
	if err != nil {
		return nil, err
	}
	return &pb.ListSellerProductsResponse{
		Products: convertToPbProducts(products),
	}, nil
}

func (s *ProductService) BatchUpsertProduct(ctx context.Context, req *pb.BatchUpsertProductRequest) (*pb.BatchUpsertProductResponse, error) {
	userID, role, err := middleware.ExtractUserInfo(&ctx)
	if err != nil {
		return nil, err
	}
	bizProducts := convertToBizProducts(req.Products)
	if err := s.uc.BatchUpsertProducts(ctx, bizProducts, req.UpdateMask, userID, biz.UserRole(role)); err != nil {
		return nil, err
	}
	return &pb.BatchUpsertProductResponse{}, nil
}
