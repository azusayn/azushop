package service

import (
	inventorypb "azushop/api/inventory/v1"
	v1 "azushop/api/inventory/v1"
	pb "azushop/api/order/v1"
	productpb "azushop/api/product/v1"
	"azushop/internal/biz"
	"azushop/internal/data"
	"azushop/internal/pkg/middleware"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

type OrderService struct {
	pb.UnimplementedOrderServiceServer
	uc   *biz.OrderUsecase
	data *data.Data
}

func NewOrderService(uc *biz.OrderUsecase, data *data.Data) *OrderService {
	return &OrderService{
		uc:   uc,
		data: data,
	}
}

func (s *OrderService) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.CreateOrderResponse, error) {
	if len(req.OrderItems) == 0 {
		return nil, errors.New("empty order_items")
	}
	userID, _, err := middleware.ExtractUserInfo(&ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	var skuIDs []string
	for _, orderItem := range req.OrderItems {
		skuIDs = append(skuIDs, orderItem.SkuId)
	}

	// fetch unit price.
	productService := s.data.GetProductService()
	m, err := fetchAllSkuDetails(ctx, productService, skuIDs)
	if err != nil {
		return nil, err
	}

	// create order.
	orderItems, err := convertToBizOrderItems(req.OrderItems, m)
	if err != nil {
		return nil, err
	}
	order, err := s.uc.CreateOrder(ctx, orderItems, userID)
	if err != nil {
		return nil, err
	}
	pbOrder, err := convertToPbOrder(order)
	if err != nil {
		return nil, err
	}

	// reserve stocks.
	var pbStockItems []*inventorypb.StockItem
	for _, orderItem := range req.OrderItems {
		pbStockItems = append(pbStockItems, &inventorypb.StockItem{
			SkuId:    orderItem.SkuId,
			Quantity: orderItem.Quantity,
		})
	}
	inventoryService := s.data.GetIventoryService()
	reserveStockReq := &v1.ReserveStockRequest{
		OrderId: order.ID,
		Items:   pbStockItems,
	}
	if _, err := inventoryService.ReserveStock(ctx, reserveStockReq); err != nil {
		// TODO(0): order in 'pending' status must be cleaned by the backend worker.
		if err := s.uc.DeleteOrder(ctx, order.ID); err != nil {
			slog.Warn(err.Error())
		}
		return nil, err
	}

	return &pb.CreateOrderResponse{Order: pbOrder}, nil
}

func (s *OrderService) CancelOrder(ctx context.Context, req *pb.CancelOrderRequest) (*pb.CancelOrderResponse, error) {
	if err := s.uc.CancelOrder(ctx, req.GetOrderId()); err != nil {
		return nil, err
	}
	inventoryService := s.data.GetIventoryService()
	// TODO(0): retrying & outbox
	_, err := inventoryService.ReleaseStock(ctx, &v1.ReleaseStockRequest{OrderId: req.OrderId})
	if err != nil {
		return nil, err
	}
	return &pb.CancelOrderResponse{}, nil
}

func (s *OrderService) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.GetOrderResponse, error) {
	order, err := s.uc.GetOrder(ctx, req.OrderId)
	if err != nil {
		return nil, err
	}
	pbOrder, err := convertToPbOrder(order)
	if err != nil {
		return nil, err
	}
	return &pb.GetOrderResponse{Order: pbOrder}, nil
}

func (s *OrderService) ListOrders(ctx context.Context, req *pb.ListOrdersRequest) (*pb.ListOrdersResponse, error) {
	if req.PageSize < 1 || req.PageSize > maxPageSize {
		return nil, status.Error(codes.OutOfRange, fmt.Sprintf("invalid page size %d", req.PageSize))
	}
	userID, _, err := middleware.ExtractUserInfo(&ctx)
	if err != nil {
		return nil, err
	}
	orders, err := s.uc.ListOrders(ctx, userID, convertToBizOrderStatus(req.OrderStatus), req.PageToken, req.PageSize)
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
	return &pb.ListOrdersResponse{
		Orders:        pbOrders,
		NextPageToken: nextPageToken,
	}, nil
}

func fetchAllSkuDetails(
	ctx context.Context,
	productService productpb.ProductServiceClient,
	skuIDs []string,
) (map[string]*productpb.SkuDetail, error) {
	var nextPageToken string
	// mapping from uuid to SkuDetail.
	m := make(map[string]*productpb.SkuDetail)
	for {
		resp, err := productService.BatchGetSkus(ctx, &productpb.BatchGetSkusRequest{
			PageToken: nextPageToken,
			PageSize:  maxPageSize,
			SkuIds:    skuIDs,
		})
		if err != nil {
			return nil, err
		}
		for _, skuDetail := range resp.SkuDetails {
			m[skuDetail.GetSku().GetId()] = skuDetail
		}
		if resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}
	return m, nil
}

func convertToPbOrderStatus(status *biz.OrderStatus) pb.OrderStatus {
	if status != nil {
		switch *status {
		case biz.OrderStatusPending:
			return pb.OrderStatus_ORDER_STATUS_PENDING
		case biz.OrderStatusPaid:
			return pb.OrderStatus_ORDER_STATUS_PAID
		case biz.OrderStatusCancelled:
			return pb.OrderStatus_ORDER_STATUS_CANCELLED
		case biz.OrderStatusRefunded:
			return pb.OrderStatus_ORDER_STATUS_REFUNDED
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
		case pb.OrderStatus_ORDER_STATUS_PAID:
			return biz.OrderStatusPaid
		case pb.OrderStatus_ORDER_STATUS_CANCELLED:
			return biz.OrderStatusCancelled
		case pb.OrderStatus_ORDER_STATUS_REFUNDED:
			return biz.OrderStatusRefunded
		default:
		}
	}
	return biz.OrderStatusUnspcified
}

// mapping: mapping from skuId to productpb.Sku
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

func convertToBizOrder(order *pb.Order) (*biz.Order, error) {
	bizOrderItems := make([]*biz.OrderItem, 0, len(order.OrderItems))
	for _, item := range order.OrderItems {
		unitPrice := ""
		if item.UnitPrice != nil {
			unitPrice = *item.UnitPrice
		}
		skuID, err := uuid.Parse(item.SkuId)
		if err != nil {
			return nil, err
		}
		priceDecimal, err := decimal.NewFromString(unitPrice)
		if err != nil {
			return nil, err
		}
		bizOrderItems = append(bizOrderItems, &biz.OrderItem{
			SkuID:     skuID,
			Quantity:  item.Quantity,
			UnitPrice: priceDecimal,
		})
	}
	orderItemsJSON, err := json.Marshal(bizOrderItems)
	if err != nil {
		return nil, err
	}
	decimalTotal, err := decimal.NewFromString(order.Total)
	if err != nil {
		return nil, err
	}
	return &biz.Order{
		ID:         order.OrderId,
		Total:      decimalTotal,
		Status:     convertToBizOrderStatus(&order.OrderStatus),
		OrderItems: orderItemsJSON,
	}, nil
}
