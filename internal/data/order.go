package data

import (
	"azushop/internal/biz"
	"context"
)

type OrderRepo struct {
	data *Data
}

func NewOrderRepo(data *Data) biz.OrderRepo {
	return &OrderRepo{data: data}
}

func (repo *OrderRepo) CreateOrder(ctx context.Context, orderItems []*biz.OrderItem, userID int32) (*biz.Order, error) {
	return nil, nil
}
