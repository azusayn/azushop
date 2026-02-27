package service

import (
	pb "azushop/api/product/v1"
	"azushop/internal/biz"
	"azushop/internal/pkg/middleware"
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
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

	pbProducts, err := convertToPbProducts(products)
	if err != nil {
		return nil, err
	}
	return &pb.ListSellerProductsResponse{
		Products: pbProducts,
	}, nil
}

// func (s *ProductService) BatchUpsertProduct(ctx context.Context, req *pb.BatchUpsertProductRequest) (*pb.BatchUpsertProductResponse, error) {
// 	userID, role, err := middleware.ExtractUserInfo(&ctx)
// 	if err != nil {
// 		return nil, err
// 	}
// 	bizProducts := convertToBizProducts(req.Products)
// 	if err := s.uc.BatchUpsertProducts(ctx, bizProducts, req.UpdateMask, userID, biz.UserRole(role)); err != nil {
// 		return nil, err
// 	}
// 	return &pb.BatchUpsertProductResponse{}, nil
// }

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

func convertToBizSkus(pbSkus []*pb.Sku) ([]*biz.Sku, error) {
	var skus []*biz.Sku
	for _, pbSku := range pbSkus {
		attrsJson, err := protojson.Marshal(pbSku.GetAttrs())
		if err != nil {
			return nil, err
		}
		skus = append(skus, &biz.Sku{
			ID:        pbSku.GetId(),
			Attrs:     attrsJson,
			UnitPrice: pbSku.GetUnitPrice(),
		})
	}
	return skus, nil
}

func convertToPbSkus(skus []*biz.Sku) ([]*pb.Sku, error) {
	var pbSkus []*pb.Sku
	for _, sku := range skus {
		var attrs structpb.Struct
		if err := protojson.Unmarshal(sku.Attrs, &attrs); err != nil {
			return nil, err
		}
		pbSkus = append(pbSkus, &pb.Sku{
			Id:        sku.ID,
			Attrs:     &attrs,
			UnitPrice: sku.UnitPrice,
		})
	}
	return pbSkus, nil
}

func convertToPbProducts(products []*biz.Product) ([]*pb.Product, error) {
	var pbProducts []*pb.Product
	for _, p := range products {
		pbSkus, err := convertToPbSkus(p.Skus)
		if err != nil {
			return nil, err
		}
		pbProducts = append(pbProducts, &pb.Product{
			Id:            p.ID,
			ProductName:   p.ProductName,
			SellerId:      p.SellerID,
			ProductStatus: convertToPbProductStatus(&p.ProductStatus),
			Skus:          pbSkus,
		})
	}
	return pbProducts, nil
}

func convertToBizProducts(pbProducts []*pb.Product) ([]*biz.Product, error) {
	var products []*biz.Product
	for _, p := range pbProducts {
		skus, err := convertToBizSkus(p.Skus)
		if err != nil {
			return nil, err
		}
		products = append(products, &biz.Product{
			ID:            p.Id,
			ProductName:   p.ProductName,
			SellerID:      p.SellerId,
			ProductStatus: convertToBizProductStatus(&p.ProductStatus),
			Skus:          skus,
		})
	}
	return products, nil
}
