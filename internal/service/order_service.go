package service

import (
	pb "azushop/api/order/v1"
	"context"
)

type OrderService struct {
	pb.UnimplementedOrderServiceServer
}

func NewOrderService() *OrderService {
	return &OrderService{}
}

func (s *OrderService) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.CreateOrderResponse, error) {

	return nil, nil
}
