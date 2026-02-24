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

func convertToPbProduct(product *biz.Product) *pb.Product {
	return &pb.Product{
		Id:            product.ID,
		ProductName:   product.ProductName,
		SellerId:      product.SellerID,
		ProductStatus: convertToPbProductStatus(&product.ProductStatus),
	}
}

func convertToPbProducts(products []*biz.Product) []*pb.Product {
	var pbProducts []*pb.Product
	for _, p := range products {
		pbProducts = append(pbProducts, convertToPbProduct(p))
	}
	return pbProducts
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
		int32(userID),
		biz.UserRole(role),
	)
	if err != nil {
		return nil, err
	}
	return &pb.ListSellerProductsResponse{
		Products: convertToPbProducts(products),
	}, nil
}

// func (s *ProductService) UpsertProduct(ctx context.Context, req *pb.UpsertProductRequest) (*pb.ListMerchantProductsResponse, error) {
// 	bizProduct := convertToBizProduct(req.Id, req.ProductName, req.SellerId, req.SkuDetails)
// 	return &pb.ListMerchantProductsResponse{}, s.uc.UpsertProduct(ctx, bizProduct, req.UpdateMask)
// }
