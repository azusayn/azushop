package service

import (
	"github.com/azusayn/azushop/internal/common"
	pb "github.com/azusayn/azushop/proto/api/inventory/v1"

	"context"
	"errors"

	"github.com/azusayn/azushop/internal/biz"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"uuid"

	"connectrpc.com/connect"
)

type InventoryService struct {
	uc *biz.InventoryUsecase
}

func NewInventoryService(uc *biz.InventoryUsecase) *InventoryService {
	return &InventoryService{uc: uc}
}

// InventoryServiceConnectHandler implements the ConnectRPC handler for InventoryService.
type InventoryServiceConnectHandler struct {
	inventoryService *InventoryService
}

func NewInventoryServiceConnectHandler(inventoryService *InventoryService) *InventoryServiceConnectHandler {
	return &InventoryServiceConnectHandler{inventoryService: inventoryService}
}

// TODO(0): RBAC
func (h *InventoryServiceConnectHandler) AdjustStock(ctx context.Context, req *connect.Request[pb.AdjustStockRequest]) (*connect.Response[pb.AdjustStockResponse], error) {
	r := req.Msg
	if r.StockQuantity < 0 {
		return nil, status.Error(codes.InvalidArgument, "stock_quantity cannot be negative")
	}
	skuId, err := uuid.Parse(r.SkuId)
	if err != nil {
		return nil, err
	}
	_, role, err := common.ExtractUserInfo(&ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	if err := h.inventoryService.uc.AdjustStock(ctx, skuId, r.StockQuantity, biz.UserRole(role)); err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.AdjustStockResponse{}), nil
}

func (h *InventoryServiceConnectHandler) BatchGetStock(ctx context.Context, req *connect.Request[pb.BatchGetStockRequest]) (*connect.Response[pb.BatchGetStockResponse], error) {
	r := req.Msg
	if len(r.SkuIds) == 0 {
		return connect.NewResponse(&pb.BatchGetStockResponse{}), nil
	}
	var uuids []uuid.UUID
	for _, skuId := range r.SkuIds {
		uuid, err := uuid.Parse(skuId)
		if err != nil {
			return nil, err
		}
		uuids = append(uuids, uuid)
	}
	inventories, err := h.inventoryService.uc.BatchGetInventories(ctx, uuids)
	if err != nil {
		return nil, err
	}
	pbInventories, err := convertToPbInventories(inventories)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.BatchGetStockResponse{
		Stocks: pbInventories,
	}), nil
}

func (h *InventoryServiceConnectHandler) ReleaseStock(ctx context.Context, req *connect.Request[pb.ReleaseStockRequest]) (*connect.Response[pb.ReleaseStockResponse], error) {
	if err := h.inventoryService.uc.ReleaseStock(ctx, req.Msg.OrderId); err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.ReleaseStockResponse{}), nil
}

func convertToPbInventories(inventories []*biz.Inventory) (map[string]*pb.SKUQuantity, error) {
	m := make(map[string]*pb.SKUQuantity)
	for _, inventory := range inventories {
		uuidStr := inventory.SkuID.String()
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
