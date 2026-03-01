package data

import (
	"azushop/internal/biz"
	"context"
	"encoding/json"

	"github.com/shopspring/decimal"
)

type OrderRepo struct {
	data *Data
}

func NewOrderRepo(data *Data) biz.OrderRepo {
	return &OrderRepo{data: data}
}

func (repo *OrderRepo) CreateOrder(
	ctx context.Context,
	orderItems []*biz.OrderItem,
	total decimal.Decimal,
	orderStatus biz.OrderStatus,
	userID int32,
) (*biz.Order, error) {
	gormClient := GetTransaction(ctx)
	if gormClient == nil {
		gormClient = repo.data.gormClient
	}
	itemsJson, err := json.Marshal(orderItems)
	if err != nil {
		return nil, err
	}
	order := &biz.Order{
		UserID:     userID,
		Status:     orderStatus,
		OrderItems: itemsJson,
		Total:      total,
	}
	if err := gormClient.WithContext(ctx).Create(order).Error; err != nil {
		return nil, err
	}
	return order, nil
}

func (repo *OrderRepo) DeleteOrder(ctx context.Context, orderID int64) error {
	gormClient := GetTransaction(ctx)
	if gormClient == nil {
		gormClient = repo.data.gormClient
	}
	return gormClient.WithContext(ctx).Where("id = ?", orderID).Delete(&biz.Order{}).Error
}
