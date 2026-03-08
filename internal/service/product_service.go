package service

import (
	pb "azushop/api/product/v1"
	"azushop/internal/biz"
	"azushop/internal/common"
	"azushop/internal/pkg/middleware"
	"context"
	"errors"

	"github.com/google/uuid"
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
	uuid, err := uuid.Parse(req.PageToken)
	if err != nil {
		return nil, err
	}
	products, err := s.uc.ListSellerProducts(
		ctx,
		req.SellerId,
		uuid,
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

func (s *ProductService) BatchCreateProduct(ctx context.Context, req *pb.BatchCreateProductRequest) (*pb.BatchCreateProductResponse, error) {
	userID, role, err := middleware.ExtractUserInfo(&ctx)
	if err != nil {
		return nil, err
	}
	products, err := convertToBizProducts(req.Products)
	if err != nil {
		return nil, err
	}
	if err := s.uc.BatchCheckProducts(products); err != nil {
		return nil, err
	}
	_, err = s.uc.BatchCreateProducts(ctx, products, userID, biz.UserRole(role))
	if err != nil {
		return nil, err
	}
	return &pb.BatchCreateProductResponse{}, nil
}

func (s *ProductService) BatchUpdateProduct(ctx context.Context, req *pb.BatchUpdateProductRequest) (*pb.BatchUpdateProductResponse, error) {
	userID, role, err := middleware.ExtractUserInfo(&ctx)
	if err != nil {
		return nil, err
	}
	if len(req.Products) == 0 {
		return nil, errors.New("empty products")
	}
	products, err := convertToBizProducts(req.Products)
	if err != nil {
		return nil, err
	}
	paths := convertToUniquePaths(req.UpdateMask)
	if err := s.uc.BatchUpdateProducts(ctx, products, paths, userID, biz.UserRole(role)); err != nil {
		return nil, err
	}
	return &pb.BatchUpdateProductResponse{}, nil
}

func (s *ProductService) BatchGetSkus(ctx context.Context, req *pb.BatchGetSkusRequest) (*pb.BatchGetSkusResponse, error) {
	if req.PageSize < 1 || req.PageSize > maxPageSize {
		return nil, status.Error(codes.OutOfRange, codes.OutOfRange.String())
	}
	var uuids []uuid.UUID
	for _, skuId := range req.SkuIds {
		u, err := uuid.Parse(skuId)
		if err != nil {
			return nil, err
		}
		uuids = append(uuids, u)
	}
	pageToken, err := common.ParseUUID(req.PageToken)
	if err != nil {
		return nil, err
	}
	skuDetails, err := s.uc.BatchGetSkuDetails(ctx, uuids, pageToken, req.PageSize)
	if err != nil {
		return nil, err
	}
	pbSkuDetails, err := convertToPbSkuDetails(skuDetails)
	if err != nil {
		return nil, err
	}
	var nextPageToken string
	lenPbSkuDetails := len(pbSkuDetails)
	if lenPbSkuDetails == int(req.PageSize) {
		nextPageToken = pbSkuDetails[lenPbSkuDetails-1].GetSku().GetId()
	}
	return &pb.BatchGetSkusResponse{
		SkuDetails:    pbSkuDetails,
		NextPageToken: nextPageToken,
	}, nil
}

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
		bytesUuid, err := common.ParseUUID(pbSku.GetId())
		if err != nil {
			return nil, err
		}
		skus = append(skus, &biz.Sku{
			ID:        bytesUuid,
			Attrs:     attrsJson,
			UnitPrice: pbSku.GetUnitPrice(),
		})
	}
	return skus, nil
}

func convertToPbSku(sku *biz.Sku) (*pb.Sku, error) {
	var attrs structpb.Struct
	if err := protojson.Unmarshal(sku.Attrs, &attrs); err != nil {
		return nil, err
	}
	pbSku := &pb.Sku{
		Id:        sku.ID.String(),
		Attrs:     &attrs,
		UnitPrice: sku.UnitPrice,
	}
	return pbSku, nil
}

func convertToPbSkuDetails(skuDetails []*biz.SkuDetail) ([]*pb.SkuDetail, error) {
	var pbSkuDetails []*pb.SkuDetail
	for _, skuDetail := range skuDetails {
		pbSku, err := convertToPbSku(&skuDetail.Sku)
		if err != nil {
			return nil, err
		}
		pbSkuDetails = append(pbSkuDetails, &pb.SkuDetail{
			Sku:         pbSku,
			ProductName: skuDetail.ProductName,
		})
	}
	return pbSkuDetails, nil
}

func convertToPbSkus(skus []*biz.Sku) ([]*pb.Sku, error) {
	var pbSkus []*pb.Sku
	for _, sku := range skus {
		pbSku, err := convertToPbSku(sku)
		if err != nil {
			return nil, err
		}
		pbSkus = append(pbSkus, pbSku)
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
			Id:            p.ID.String(),
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
		bytesUuid, err := common.ParseUUID(p.Id)
		if err != nil {
			return nil, err
		}
		products = append(products, &biz.Product{
			ID:            bytesUuid,
			ProductName:   p.ProductName,
			SellerID:      p.SellerId,
			ProductStatus: convertToBizProductStatus(&p.ProductStatus),
			Skus:          skus,
		})
	}
	return products, nil
}
