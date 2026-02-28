package service

import (
	pb "azushop/api/order/v1"
	productpb "azushop/api/product/v1"
	"azushop/internal/biz"
	"azushop/internal/data"
	"azushop/internal/pkg/middleware"
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

	productService := s.data.GetProductService()
	var nextPageToken string
	m := make(map[string]*productpb.Sku)
	for {
		resp, err := productService.BatchGetSkus(ctx, &productpb.BatchGetSkusRequest{
			PageToken: nextPageToken,
			PageSize:  100,
			SkuIds:    skuIDs,
		})
		if err != nil {
			return nil, err
		}
		for _, sku := range resp.Skus {
			m[sku.Id] = sku
		}
		if resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}

	for _, orderItem := range req.OrderItems {
		orderItem.UnitPrice = &m[orderItem.SkuId].UnitPrice
	}
	orderItems, err := convertToBizOrderItems(req.OrderItems)
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
	return &pb.CreateOrderResponse{Order: pbOrder}, nil
}

func convertToPbOrderStatus(status string) pb.OrderStatus {
	switch status {
	case biz.OrderStatusPending:
		return pb.OrderStatus_ORDER_STATUS_PENDING
	case biz.OrderStatusPaid:
		return pb.OrderStatus_ORDER_STATUS_PAID
	case biz.OrderStatusCancelled:
		return pb.OrderStatus_ORDER_STATUS_CANCELLED
	case biz.OrderStatusRefunded:
		return pb.OrderStatus_ORDER_STATUS_REFUNDED
	default:
		return pb.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}

func convertToBizOrderStatus(status pb.OrderStatus) string {
	switch status {
	case pb.OrderStatus_ORDER_STATUS_PENDING:
		return biz.OrderStatusPending
	case pb.OrderStatus_ORDER_STATUS_PAID:
		return biz.OrderStatusPaid
	case pb.OrderStatus_ORDER_STATUS_CANCELLED:
		return biz.OrderStatusCancelled
	case pb.OrderStatus_ORDER_STATUS_REFUNDED:
		return biz.OrderStatusRefunded
	default:
		return biz.OrderStatusUnspcified
	}
}

func convertToBizOrderItems(pbOrderItems []*pb.OrderItem) ([]*biz.OrderItem, error) {
	var orderItems []*biz.OrderItem
	for _, pbOrderItem := range pbOrderItems {
		uuid, err := uuid.Parse(pbOrderItem.SkuId)
		if err != nil {
			return nil, err
		}
		orderItems = append(orderItems, &biz.OrderItem{
			SkuID:     uuid,
			Quantity:  pbOrderItem.GetQuantity(),
			UnitPrice: pbOrderItem.GetUnitPrice(),
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
		pbOrderItems = append(pbOrderItems, &pb.OrderItem{
			SkuId:     item.SkuID.String(),
			Quantity:  item.Quantity,
			UnitPrice: &item.UnitPrice,
		})
	}
	return &pb.Order{
		OrderId:     order.ID,
		Total:       order.Total,
		OrderStatus: convertToPbOrderStatus(order.Status),
		OrderItems:  pbOrderItems,
	}, nil
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
		bizOrderItems = append(bizOrderItems, &biz.OrderItem{
			SkuID:     skuID,
			Quantity:  item.Quantity,
			UnitPrice: unitPrice,
		})
	}
	orderItemsJSON, err := json.Marshal(bizOrderItems)
	if err != nil {
		return nil, err
	}
	return &biz.Order{
		ID:         order.OrderId,
		Total:      order.Total,
		Status:     convertToBizOrderStatus(order.OrderStatus),
		OrderItems: orderItemsJSON,
	}, nil
}
