package service

import (
	"github.com/azusayn/azushop/internal/biz"
	"github.com/azusayn/azushop/internal/common"

	pb "github.com/azusayn/azushop/proto/api/product/v1"
	productv1connect "github.com/azusayn/azushop/proto/api/product/v1/v1connect"
	"github.com/azusayn/azushop/proto/conf"

	"context"
	"errors"

	"github.com/azusayn/azushop/internal/pkg/str"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"connectrpc.com/connect"
)

type ProductService struct {
	uc                         *biz.ProductUsecase
	maxEmbeddingSearchDistance float32
}

func NewProductService(uc *biz.ProductUsecase, cd *conf.Data) *ProductService {
	return &ProductService{
		uc:                         uc,
		maxEmbeddingSearchDistance: cd.GetEmbeddingApi().GetMaxDistance(),
	}
}

const maxPageSizeProduct = 100

// ProductServiceConnectHandler implements the ConnectRPC handler for ProductService.
type ProductServiceConnectHandler struct {
	productv1connect.UnimplementedProductServiceHandler
	productService *ProductService
}

func NewProductServiceConnectHandler(productService *ProductService) *ProductServiceConnectHandler {
	return &ProductServiceConnectHandler{productService: productService}
}

func (h *ProductServiceConnectHandler) SearchProducts(ctx context.Context, req *connect.Request[pb.SearchProductsRequest]) (*connect.Response[pb.SearchProductsResponse], error) {
	r := req.Msg
	if r.PageSize > maxPageSizeProduct {
		return nil, status.Error(codes.OutOfRange, codes.OutOfRange.String())
	}
	pageToken, err := str.ParseUUID(r.PageToken)
	if err != nil {
		return nil, err
	}
	products, err := h.productService.uc.SearchProducts(
		ctx,
		r.GetKeyword(),
		h.productService.maxEmbeddingSearchDistance,
		pageToken,
		r.GetPageSize(),
	)
	if err != nil {
		return nil, err
	}

	pbProducts, err := convertToPbProducts(products)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&pb.SearchProductsResponse{
		Products: pbProducts,
	}), nil
}

func (h *ProductServiceConnectHandler) ListSellerProducts(ctx context.Context, req *connect.Request[pb.ListSellerProductsRequest]) (*connect.Response[pb.ListSellerProductsResponse], error) {
	r := req.Msg
	if r.PageSize > maxPageSizeProduct {
		return nil, status.Error(codes.OutOfRange, codes.OutOfRange.String())
	}
	userID, role, err := common.ExtractUserInfo(&ctx)
	if err != nil {
		return nil, err
	}
	uuid, err := str.ParseUUID(r.PageToken)
	if err != nil {
		return nil, err
	}
	products, err := h.productService.uc.ListSellerProducts(
		ctx,
		r.SellerId,
		uuid,
		r.PageSize,
		convertToBizProductStatus(r.ProductStatus),
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
	return connect.NewResponse(&pb.ListSellerProductsResponse{
		Products: pbProducts,
	}), nil
}

func (h *ProductServiceConnectHandler) BatchCreateProduct(ctx context.Context, req *connect.Request[pb.BatchCreateProductRequest]) (*connect.Response[pb.BatchCreateProductResponse], error) {
	r := req.Msg
	userID, role, err := common.ExtractUserInfo(&ctx)
	if err != nil {
		return nil, err
	}
	products, err := convertToBizProducts(r.Products)
	if err != nil {
		return nil, err
	}
	if err := h.productService.uc.BatchCheckProducts(products); err != nil {
		return nil, err
	}

	_, err = h.productService.uc.BatchCreateProducts(ctx, products, userID, biz.UserRole(role))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.BatchCreateProductResponse{}), nil
}

func (h *ProductServiceConnectHandler) BatchUpdateProduct(ctx context.Context, req *connect.Request[pb.BatchUpdateProductRequest]) (*connect.Response[pb.BatchUpdateProductResponse], error) {
	r := req.Msg
	userID, role, err := common.ExtractUserInfo(&ctx)
	if err != nil {
		return nil, err
	}
	if len(r.Products) == 0 {
		return nil, errors.New("empty products")
	}
	products, err := convertToBizProducts(r.Products)
	if err != nil {
		return nil, err
	}
	paths := convertToUniquePaths(r.UpdateMask)
	if err := h.productService.uc.BatchUpdateProducts(ctx, products, paths, userID, biz.UserRole(role)); err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.BatchUpdateProductResponse{}), nil
}

func (h *ProductServiceConnectHandler) BatchGetSkus(ctx context.Context, req *connect.Request[pb.BatchGetSkusRequest]) (*connect.Response[pb.BatchGetSkusResponse], error) {
	r := req.Msg
	if r.PageSize < 1 || r.PageSize > maxPageSizeProduct {
		return nil, status.Error(codes.OutOfRange, codes.OutOfRange.String())
	}
	var uuids []uuid.UUID
	if len(r.SkuIds) == 0 {
		return nil, errors.New("empty sku IDs")
	}
	for _, skuId := range r.SkuIds {
		u, err := uuid.Parse(skuId)
		if err != nil {
			return nil, err
		}
		uuids = append(uuids, u)
	}
	pageToken, err := str.ParseUUID(r.PageToken)
	if err != nil {
		return nil, err
	}
	skuDetails, err := h.productService.uc.BatchGetSkuDetails(ctx, uuids, pageToken, r.PageSize)
	if err != nil {
		return nil, err
	}
	pbSkuDetails, err := convertToPbSkuDetails(skuDetails)
	if err != nil {
		return nil, err
	}
	var nextPageToken string
	lenPbSkuDetails := len(pbSkuDetails)
	if lenPbSkuDetails == int(r.PageSize) {
		nextPageToken = pbSkuDetails[lenPbSkuDetails-1].GetSku().GetId()
	}
	return connect.NewResponse(&pb.BatchGetSkusResponse{
		SkuDetails:    pbSkuDetails,
		NextPageToken: nextPageToken,
	}), nil
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
		bytesUuid, err := str.ParseUUID(pbSku.GetId())
		if err != nil {
			return nil, err
		}
		skus = append(skus, &biz.Sku{
			ID:        bytesUuid,
			Attrs:     attrsJson,
			UnitPrice: biz.Numeric(pbSku.GetUnitPrice()),
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
		UnitPrice: string(sku.UnitPrice),
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
		bytesUuid, err := str.ParseUUID(p.Id)
		if err != nil {
			return nil, err
		}
		products = append(products, &biz.Product{
			ID:            bytesUuid,
			ProductName:   p.ProductName,
			SellerID:      p.SellerId,
			Description:   p.Description,
			ProductStatus: convertToBizProductStatus(&p.ProductStatus),
			Skus:          skus,
		})
	}
	return products, nil
}
