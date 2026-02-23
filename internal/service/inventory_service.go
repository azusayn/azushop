package service

import (
	pb "azushop/api/inventory/v1"
	"azushop/internal/biz"
	"context"
	"encoding/json"

	"google.golang.org/protobuf/types/known/structpb"
)

type InventoryService struct {
	pb.UnimplementedInventoryServiceServer
	uc *biz.InventoryUsecase
}

func NewInventoryService(uc *biz.InventoryUsecase) *InventoryService {
	return &InventoryService{uc: uc}
}

func convertToPbMerchantSkus(skus []*biz.Sku) []*pb.MerchantSku {
	var pbMerchantSkus []*pb.MerchantSku
	for _, sku := range skus {
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
	return pbMerchantSkus
}

func convertToPbSkus(skus []*biz.Sku) []*pb.Sku {
	var pbSkus []*pb.Sku
	for _, sku := range skus {
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
	return pbSkus
}

func convertToBizSkus(pbSkus []*pb.SkuDetail) []*biz.Sku {
	var skus []*biz.Sku
	for _, pbSku := range pbSkus {
		skus = append(skus, &biz.Sku{
			ID:        pbSku.GetId(),
			Attrs:     json.RawMessage(pbSku.GetAttrs().String()),
			UnitPrice: pbSku.GetUnitPrice(),
		})
	}
	return skus
}

func (s *InventoryService) ListSKUs(ctx context.Context, req *pb.ListSKUsRequest) (*pb.ListSKUsResponse, error) {
	skus, err := s.uc.ListSKUs(ctx, req.ProductId)
	if err != nil {
		return nil, err
	}
	return &pb.ListSKUsResponse{
		Skus: convertToPbSkus(skus),
	}, nil
}

func (s *InventoryService) AdminListSKUs(ctx context.Context, req *pb.AdminListSKUsRequest) (*pb.AdminListSKUsResponse, error) {
	skus, err := s.uc.ListSKUs(ctx, req.ProductId)
	if err != nil {
		return nil, err
	}
	return &pb.AdminListSKUsResponse{
		Skus: convertToPbMerchantSkus(skus),
	}, nil
}

func (s *InventoryService) UpdateSKU(ctx context.Context, req *pb.UpdateSKURequest) (*pb.UpdateSKUResponse, error) {
	err := s.uc.UpdateSKU(ctx, convertToBizSkus(req.SkuDetails))
	if err != nil {
		return nil, err
	}
	return &pb.UpdateSKUResponse{}, nil
}

func (s *InventoryService) CreateSKU(ctx context.Context, req *pb.CreateSKURequest) (*pb.CreateSKUResponse, error) {
	err := s.uc.CreateSKU(ctx, req.ProductId, convertToBizSkus(req.SkuDetails))
	if err != nil {
		return nil, err
	}
	return &pb.CreateSKUResponse{}, nil
}
