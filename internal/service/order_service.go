package service

import (
	inventorypb "github.com/azusayn/azushop/proto/api/inventory/v1"
	pb "github.com/azusayn/azushop/proto/api/order/v1"
	productpb "github.com/azusayn/azushop/proto/api/product/v1"

	"github.com/azusayn/azushop/internal/biz"
	"github.com/azusayn/azushop/internal/common"

	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"connectrpc.com/connect"

	inventoryv1connect "github.com/azusayn/azushop/proto/api/inventory/v1/v1connect"
	productv1connect "github.com/azusayn/azushop/proto/api/product/v1/v1connect"
)

type OrderService struct {
	uc        *biz.OrderUsecase
	product   productv1connect.ProductServiceClient
	inventory inventoryv1connect.InventoryServiceClient
}

func NewOrderService(
	uc *biz.OrderUsecase,
	product productv1connect.ProductServiceClient,
	inventory inventoryv1connect.InventoryServiceClient,
) *OrderService {
	return &OrderService{
		uc:        uc,
		product:   product,
		inventory: inventory,
	}
}

const maxPageSize = 100

// OrderServiceConnectHandler implements the ConnectRPC handler for OrderService.
type OrderServiceConnectHandler struct {
	orderService *OrderService
}

func NewOrderServiceConnectHandler(orderService *OrderService) *OrderServiceConnectHandler {
	return &OrderServiceConnectHandler{orderService: orderService}
}

func (h *OrderServiceConnectHandler) CreateOrder(ctx context.Context, req *connect.Request[pb.CreateOrderRequest]) (*connect.Response[pb.CreateOrderResponse], error) {
	idempotencyKey, err := common.ExtractIdempotencyKey(&ctx)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	r := req.Msg
	if len(r.OrderItems) == 0 {
		return nil, errors.New("empty order_items")
	}
	userID, _, err := common.ExtractUserInfo(&ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	skuIDs := lo.Map(r.OrderItems, func(item *pb.OrderItem, index int) string { return item.SkuId })

	m, err := fetchAllSkuDetails(ctx, h.orderService.product, skuIDs)
	if err != nil {
		return nil, err
	}

	orderItems, err := convertToBizOrderItems(r.OrderItems, m)
	if err != nil {
		return nil, err
	}
	order, err := h.orderService.uc.CreateOrder(ctx, idempotencyKey, orderItems, userID)
	if err != nil {
		return nil, err
	}
	pbOrder, err := convertToPbOrder(order)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&pb.CreateOrderResponse{Order: pbOrder}), nil
}

func (h *OrderServiceConnectHandler) CancelOrder(ctx context.Context, req *connect.Request[pb.CancelOrderRequest]) (*connect.Response[pb.CancelOrderResponse], error) {
	r := req.Msg
	if err := h.orderService.uc.CancelOrder(ctx, r.GetOrderId()); err != nil {
		return nil, err
	}

	releaseReq := connect.NewRequest(&inventorypb.ReleaseStockRequest{OrderId: r.OrderId})
	// TODO: outbox
	if _, err := h.orderService.inventory.ReleaseStock(ctx, releaseReq); err != nil {
		return nil, err
	}

	return connect.NewResponse(&pb.CancelOrderResponse{}), nil
}

func (h *OrderServiceConnectHandler) GetOrder(ctx context.Context, req *connect.Request[pb.GetOrderRequest]) (*connect.Response[pb.GetOrderResponse], error) {
	r := req.Msg
	order, err := h.orderService.uc.GetOrder(ctx, r.OrderId)
	if err != nil {
		return nil, err
	}
	pbOrder, err := convertToPbOrder(order)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.GetOrderResponse{Order: pbOrder}), nil
}

func (h *OrderServiceConnectHandler) ListOrders(ctx context.Context, req *connect.Request[pb.ListOrdersRequest]) (*connect.Response[pb.ListOrdersResponse], error) {
	r := req.Msg
	if r.PageSize < 1 || r.PageSize > maxPageSize {
		return nil, status.Error(codes.OutOfRange, fmt.Sprintf("invalid page size %d", r.PageSize))
	}
	userID, _, err := common.ExtractUserInfo(&ctx)
	if err != nil {
		return nil, err
	}
	orders, err := h.orderService.uc.ListOrders(ctx, userID, convertToBizOrderStatus(r.OrderStatus), r.PageToken, r.PageSize)
	if err != nil {
		return nil, err
	}
	pbOrders, err := convertToPbOrders(orders)
	if err != nil {
		return nil, err
	}
	nextPageToken := int64(0)
	lenPbOrders := len(pbOrders)
	if lenPbOrders != 0 {
		nextPageToken = pbOrders[lenPbOrders-1].OrderId
	}
	return connect.NewResponse(&pb.ListOrdersResponse{
		Orders:        pbOrders,
		NextPageToken: nextPageToken,
	}), nil
}

func fetchAllSkuDetails(
	ctx context.Context,
	productService productv1connect.ProductServiceClient,
	skuIDs []string,
) (map[string]*productpb.SkuDetail, error) {
	var nextPageToken string
	m := make(map[string]*productpb.SkuDetail)
	for {
		req := connect.NewRequest(&productpb.BatchGetSkusRequest{
			PageToken: nextPageToken,
			PageSize:  maxPageSize,
			SkuIds:    skuIDs,
		})
		resp, err := productService.BatchGetSkus(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, skuDetail := range resp.Msg.SkuDetails {
			m[skuDetail.GetSku().GetId()] = skuDetail
		}
		if resp.Msg.NextPageToken == "" {
			break
		}
		nextPageToken = resp.Msg.NextPageToken
	}
	return m, nil
}

func convertToPbOrderStatus(status *biz.OrderStatus) pb.OrderStatus {
	if status != nil {
		switch *status {
		case biz.OrderStatusPending:
			return pb.OrderStatus_ORDER_STATUS_PENDING
		case biz.OrderStatusCancelled:
			return pb.OrderStatus_ORDER_STATUS_CANCELLED
		case biz.OrderStatusConfirmed:
			return pb.OrderStatus_ORDER_STATUS_CONFIRMED
		case biz.OrderStatusCompleted:
			return pb.OrderStatus_ORDER_STATUS_COMPLETED
		default:
		}
	}
	return pb.OrderStatus_ORDER_STATUS_UNSPECIFIED
}

func convertToBizOrderStatus(status *pb.OrderStatus) biz.OrderStatus {
	if status != nil {
		switch *status {
		case pb.OrderStatus_ORDER_STATUS_PENDING:
			return biz.OrderStatusPending
		case pb.OrderStatus_ORDER_STATUS_CANCELLED:
			return biz.OrderStatusCancelled
		case pb.OrderStatus_ORDER_STATUS_CONFIRMED:
			return biz.OrderStatusConfirmed
		case pb.OrderStatus_ORDER_STATUS_COMPLETED:
			return biz.OrderStatusCompleted
		default:
		}
	}
	return biz.OrderStatusUnspecified
}

func convertToBizOrderItems(pbOrderItems []*pb.OrderItem, mapping map[string]*productpb.SkuDetail) ([]*biz.OrderItem, error) {
	var orderItems []*biz.OrderItem
	for _, pbOrderItem := range pbOrderItems {
		skuDetail, ok := mapping[pbOrderItem.SkuId]
		if !ok {
			return nil, errors.New("failed to get sku from mapping")
		}
		uuid, err := uuid.Parse(pbOrderItem.SkuId)
		if err != nil {
			return nil, err
		}
		unitPriceDecimal, err := decimal.NewFromString(skuDetail.GetSku().GetUnitPrice())
		if err != nil {
			return nil, err
		}
		bytesAttrs, err := protojson.Marshal(skuDetail.GetSku().GetAttrs())
		if err != nil {
			return nil, err
		}
		orderItems = append(orderItems, &biz.OrderItem{
			ProductName: skuDetail.GetProductName(),
			SkuID:       uuid,
			Quantity:    pbOrderItem.GetQuantity(),
			UnitPrice:   unitPriceDecimal,
			Attrs:       bytesAttrs,
		})
	}
	return orderItems, nil
}

func convertToPbOrder(order *biz.Order) (*pb.Order, error) {
	var orderItems []*biz.OrderItem
	if err := json.Unmarshal(order.OrderItems, &orderItems); err != nil {
		return nil, err
	}
	pbOrderItems := make([]*pb.OrderItem, 0, len(orderItems))
	for _, item := range orderItems {
		unitPriceStr := item.UnitPrice.String()
		var attrs structpb.Struct
		if err := json.Unmarshal(item.Attrs, &attrs); err != nil {
			return nil, err
		}
		pbOrderItems = append(pbOrderItems, &pb.OrderItem{
			SkuId:     item.SkuID.String(),
			Quantity:  item.Quantity,
			UnitPrice: &unitPriceStr,
			Attrs:     &attrs,
		})
	}
	return &pb.Order{
		OrderId:     order.ID,
		Total:       order.Total.String(),
		OrderStatus: convertToPbOrderStatus(&order.Status),
		OrderItems:  pbOrderItems,
	}, nil
}

func convertToPbOrders(orders []*biz.Order) ([]*pb.Order, error) {
	pbOrders := make([]*pb.Order, 0, len(orders))
	for _, order := range orders {
		pbOrder, err := convertToPbOrder(order)
		if err != nil {
			return nil, err
		}
		pbOrders = append(pbOrders, pbOrder)
	}
	return pbOrders, nil
}
