package service

import (
	pb "azushop/api/inventory/v1"
	"azushop/internal/biz"
	"azushop/internal/pkg/middleware"
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type InventoryService struct {
	pb.UnimplementedInventoryServiceServer
	uc *biz.InventoryUsecase
}

func NewInventoryService(uc *biz.InventoryUsecase) *InventoryService {
	return &InventoryService{uc: uc}
}

// TODO: RBAC(0)
func (s *InventoryService) AdjustStock(ctx context.Context, req *pb.AdjustStockRequest) (*pb.AdjustStockResponse, error) {
	if req.StockQuantity < 0 {
		return nil, status.Error(codes.InvalidArgument, "stock_quantity cannot be negative")
	}
	skuId, err := uuid.Parse(req.SkuId)
	if err != nil {
		return nil, err
	}
	_, role, err := middleware.ExtractUserInfo(&ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	if err := s.uc.AdjustStock(ctx, skuId, req.StockQuantity, biz.UserRole(role)); err != nil {
		return nil, err
	}
	return &pb.AdjustStockResponse{}, nil
}

func (s *InventoryService) BatchGetStock(ctx context.Context, req *pb.BatchGetStockRequest) (*pb.BatchGetStockResponse, error) {
	if len(req.SkuIds) == 0 {
		return &pb.BatchGetStockResponse{}, nil
	}
	var uuids []uuid.UUID
	for _, skuId := range req.SkuIds {
		uuid, err := uuid.Parse(skuId)
		if err != nil {
			return nil, err
		}
		uuids = append(uuids, uuid)
	}
	inventories, err := s.uc.BatchGetInventories(ctx, uuids)
	if err != nil {
		return nil, err
	}
	pbInventories, err := convertToPbInventories(inventories)
	if err != nil {
		return nil, err
	}
	return &pb.BatchGetStockResponse{
		Stocks: pbInventories,
	}, nil
}

func (s *InventoryService) ReserveStock(ctx context.Context, req *pb.ReserveStockRequest) (*pb.ReserveStockResponse, error) {
	items := req.GetItems()
	if len(items) == 0 {
		return nil, status.Error(codes.InvalidArgument, "empty items")
	}
	m := make(map[uuid.UUID]int64)
	for _, item := range req.Items {
		uuid, err := uuid.Parse(item.SkuId)
		if err != nil {
			return nil, err
		}
		if _, ok := m[uuid]; ok {
			return nil, errors.New("duplicate SkuIds")
		}
		m[uuid] = item.Quantity
	}
	if err := s.uc.ReserveStock(ctx, req.OrderId, m); err != nil {
		return nil, err
	}
	return &pb.ReserveStockResponse{}, nil
}

func (s *InventoryService) ReleaseStock(ctx context.Context, req *pb.ReleaseStockRequest) (*pb.ReleaseStockResponse, error) {
	if err := s.uc.ReleaseStock(ctx, req.OrderId); err != nil {
		return nil, err
	}
	return &pb.ReleaseStockResponse{}, nil
}

func (s *InventoryService) DeductStock(ctx context.Context, req *pb.DeductStockRequest) (*pb.DeductStockResponse, error) {
	if err := s.uc.DeductStock(ctx, req.OrderId); err != nil {
		return nil, err
	}
	return &pb.DeductStockResponse{}, nil
}

func convertToPbInventories(inventories []*biz.Inventory) (map[string]*pb.SKUQuantity, error) {
	// mapping from SkuID to pb.SKUQuantity
	m := make(map[string]*pb.SKUQuantity)
	for _, inventory := range inventories {
		uuidStr := inventory.ID.String()
		if _, ok := m[uuidStr]; ok {
			return nil, errors.New("duplicate inventories")
		}
		m[uuidStr] = &pb.SKUQuantity{
			AvailableQuantity: inventory.StockQuantity - inventory.ReservedQuantity,
			StockQuantity:     &inventory.StockQuantity,
			ReservedQuantity:  &inventory.ReservedQuantity,
		}
	}
	return m, nil
}
