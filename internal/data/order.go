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

func (repo *OrderRepo) ListOrders(
	ctx context.Context,
	userID int32,
	status biz.OrderStatus,
	pageToken int64,
	pageSize int32,
) ([]*biz.Order, error) {
	client := repo.data.gormClient
	var orders []*biz.Order
	client = client.WithContext(ctx).Where("user_id = ?", userID).Where("id > ?", pageToken)
	if status != biz.OrderStatusUnspcified {
		client = client.Where("status = ?", status)
	}
	if err := client.Limit(int(pageSize)).Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

func (repo *OrderRepo) CreateOrder(
	ctx context.Context,
	orderItems []*biz.OrderItem,
	total decimal.Decimal,
	orderStatus biz.OrderStatus,
	userID int32,
) (*biz.Order, error) {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.data.gormClient
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
	if err := client.WithContext(ctx).Create(order).Error; err != nil {
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

func (repo *OrderRepo) CancelOrder(ctx context.Context, orderID int64) error {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.data.gormClient
	}
	return client.WithContext(ctx).Where("id = ?", orderID).Update("status", biz.OrderStatusCancelled).Error
}
