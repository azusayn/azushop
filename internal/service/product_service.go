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
		var attrs structpb.Struct
		if err := attrs.UnmarshalJSON(sku.Attrs); err != nil {
			continue
		}
		pbSkus = append(pbSkus, &pb.Sku{
			SkuDetail: &pb.SkuDetail{
				Id:        sku.ID,
				Attrs:     &attrs,
				UnitPrice: sku.UnitPrice,
			},
			AvailableQuantity: sku.StockQuantity - sku.ReservedQuantity,
		})
	}
	return &pb.Product{
		Id:          product.ID,
		ProductName: product.ProductName,
		SellerId:    product.SellerID,
		Skus:        pbSkus,
	}
}

func convertToPbProducts(products []*biz.Product) []*pb.Product {
	var pbProducts []*pb.Product
	for _, p := range products {
		pbProducts = append(pbProducts, convertToPbProduct(p))
	}
	return pbProducts
}

func convertToPbMerchantProduct(product *biz.Product) *pb.MerchantProduct {
	var pbMerchantSkus []*pb.MerchantSku
	for _, sku := range product.Skus {
		var attrs structpb.Struct
		if err := attrs.UnmarshalJSON(sku.Attrs); err != nil {
			continue
		}
		pbMerchantSkus = append(pbMerchantSkus, &pb.MerchantSku{
			SkuDetail: &pb.SkuDetail{
				Id:        sku.ID,
				Attrs:     &attrs,
				UnitPrice: sku.UnitPrice,
			},
			StockQuantity:    sku.StockQuantity,
			ReservedQuantity: sku.ReservedQuantity,
		})
	}
	return &pb.MerchantProduct{
		Id:          product.ID,
		ProductName: product.ProductName,
		SellerId:    product.SellerID,
		Skus:        pbMerchantSkus,
	}
}

func convertToPbMerchantProducts(products []*biz.Product) []*pb.MerchantProduct {
	var pbMerchantProducts []*pb.MerchantProduct
	for _, p := range products {
		pbMerchantProducts = append(pbMerchantProducts, convertToPbMerchantProduct(p))
	}
	return pbMerchantProducts
}

func convertToBizProduct(productID int64, productName string, sellerID int32, skuDetails []*pb.SkuDetail) *biz.Product {
	var skus []*biz.Sku
	for _, s := range skuDetails {
		skus = append(skus, &biz.Sku{
			ID:        s.GetId(),
			ProductID: productID,
			Attrs:     json.RawMessage(s.GetAttrs().String()),
			UnitPrice: s.GetUnitPrice(),
		})
	}
	return &biz.Product{
		ID:          productID,
		ProductName: productName,
		SellerID:    sellerID,
		Skus:        skus,
	}
}

func (s *ProductService) ListProductsBySellerId(ctx context.Context, req *pb.ListProductsBySellerIdRequest) (*pb.ListProductsBySellerIdResponse, error) {
	if req.PageSize > maxPageSize {
		return nil, status.Error(codes.OutOfRange, codes.OutOfRange.String())
	}
	products, err := s.uc.ListProductsBySellerId(ctx, req.SellerId, req.PageToken, req.PageSize)
	if err != nil {
		return nil, err
	}
	return &pb.ListProductsBySellerIdResponse{
		Products: convertToPbProducts(products),
	}, nil
}

func (s *ProductService) ListMerchantProducts(ctx context.Context, req *pb.ListMerchantProductsRequest) (*pb.ListMerchantProductsResponse, error) {
	if req.PageSize > maxPageSize {
		return nil, status.Error(codes.OutOfRange, codes.OutOfRange.String())
	}
	// TODO: extract seller id from metadata.
	products, err := s.uc.ListProductsBySellerId(ctx, 0x0, req.PageToken, req.PageSize)
	if err != nil {
		return nil, err
	}
	return &pb.ListMerchantProductsResponse{
		Products: convertToPbMerchantProducts(products),
	}, nil
}

func (s *ProductService) UpsertProduct(ctx context.Context, req *pb.UpsertProductRequest) (*pb.ListMerchantProductsResponse, error) {
	bizProduct := convertToBizProduct(req.Id, req.ProductName, req.SellerId, req.SkuDetails)
	return &pb.ListMerchantProductsResponse{}, s.uc.UpsertProduct(ctx, bizProduct, req.UpdateMask)
}
