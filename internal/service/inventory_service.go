package service

import (
	pb "azushop/api/inventory/v1"
	"azushop/internal/biz"
)

type InventoryService struct {
	pb.UnimplementedInventoryServiceServer
	uc *biz.InventoryUsecase
}

func NewInventoryService(uc *biz.InventoryUsecase) *InventoryService {
	return &InventoryService{uc: uc}
}
