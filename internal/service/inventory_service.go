package service

import (
	pb "azushop/api/inventory/v1"
	"azushop/internal/biz"
	"context"

	"google.golang.org/protobuf/types/known/structpb"
)

type InventoryService struct {
	pb.UnimplementedInventoryServiceServer
	uc *biz.InventoryUsecase
}

func NewInventoryService(uc *biz.InventoryUsecase) *InventoryService {
	return &InventoryService{uc: uc}
}

func convertToPbMerchantSkus(skus []*biz.Inventory) []*pb.MerchantSku {
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
