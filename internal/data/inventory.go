package data

import (
	"azushop/internal/biz"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type InventoryRepo struct {
	data *Data
}

func NewInventoryRepo(data *Data) biz.InventoryRepo {
	return &InventoryRepo{data: data}
}

// updates the same set of fields (defined by paths) for each inventory.
func (repo *InventoryRepo) UpdateInventories(ctx context.Context, inventories []*biz.Inventory, paths []string) error {
	gormClient := GetTransaction(ctx)
	if gormClient == nil {
		gormClient = repo.data.gormClient.WithContext(ctx)
	}
	for _, inv := range inventories {
		vals := map[string]interface{}{}
		for _, path := range paths {
			switch path {
			case "stock_quantity":
				vals["stock_quantity"] = inv.StockQuantity
			case "reserved_quantity":
				vals["reserved_quantity"] = inv.ReservedQuantity
			}
		}
		if len(vals) == 0 {
			return errors.New("no valid paths")
		}
		if err := gormClient.Model(inv).Updates(vals).Error; err != nil {
			return err
		}
	}
	return nil
}

func (repo *InventoryRepo) BatchGetInventories(ctx context.Context, skuIDs []uuid.UUID) ([]*biz.Inventory, error) {
	if len(skuIDs) == 0 {
		return nil, errors.New("empty skuIDs")
	}
	gormClient := repo.data.gormClient
	var inventories []*biz.Inventory
	err := gormClient.WithContext(ctx).Where("sku_id IN ?", skuIDs).Find(&inventories).Error
	if err != nil {
		return nil, err
	}
	return inventories, nil
}

func (repo *InventoryRepo) UpdateDeltaQuantity(ctx context.Context, inventories []*biz.Inventory, paths []string) error {
	gormClient := GetTransaction(ctx)
	if gormClient == nil {
		gormClient = repo.data.gormClient
	}
	for _, inventory := range inventories {
		m := make(map[string]any)
		for _, path := range paths {
			switch path {
			case "stock_quantity":
				m[path] = gorm.Expr("stock_quantity + ?", inventory.StockQuantity)
			case "reserved_quantity":
				m[path] = gorm.Expr("reserved_quantity + ?", inventory.ReservedQuantity)
			default:
				return fmt.Errorf("invalid path %q", path)
			}
		}
		if err := gormClient.
			WithContext(ctx).
			Model(&biz.Inventory{}).
			Where("sku_id = ?", inventory.SkuID).
			Updates(m).Error; err != nil {
			return err
		}
	}
	return nil
}

func (repo *InventoryRepo) GetInventoryLock(ctx context.Context, orderID int64) (*biz.InventoryLock, error) {
	gormClient := GetTransaction(ctx)
	if gormClient == nil {
		gormClient = repo.data.gormClient
	}
	var inventoryLock biz.InventoryLock
	if err := gormClient.WithContext(ctx).Where("order_id = ?", orderID).Find(&inventoryLock).Error; err != nil {
		return nil, err
	}
	return &inventoryLock, nil
}

func (repo *InventoryRepo) CreateInventoryLock(
	ctx context.Context,
	orderID int64,
	orderItems []*biz.OrderItem,
	status biz.InventoryLockStatus,
) error {
	gormClient := GetTransaction(ctx)
	if gormClient == nil {
		gormClient = repo.data.gormClient
	}
	payload, err := json.Marshal(orderItems)
	if err != nil {
		return err
	}
	lock := &biz.InventoryLock{
		OrderID: orderID,
		Payload: payload,
		Status:  status,
	}
	return gormClient.WithContext(ctx).Table("inventory_lock").Create(lock).Error
}

func (repo *InventoryRepo) UpdateInventoryLock(ctx context.Context, inventoryLocks []*biz.InventoryLock, paths []string) error {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.data.gormClient
	}
	for _, invLock := range inventoryLocks {
		if err := client.
			WithContext(ctx).
			Model(invLock).
			Select(paths).
			Updates(invLock).Error; err != nil {
			return err
		}
	}
	return nil
}

func (repo *InventoryRepo) BatchCreateInventoris(ctx context.Context, skuIDs []uuid.UUID) ([]*biz.Inventory, error) {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.data.gormClient
	}
	var inventories []*biz.Inventory
	for _, skuID := range skuIDs {
		inventories = append(inventories, &biz.Inventory{
			SkuID:            skuID,
			StockQuantity:    0,
			ReservedQuantity: 0,
		})
	}
	if err := client.WithContext(ctx).Create(&inventories).Error; err != nil {
		return nil, err
	}
	return inventories, nil
}

type InventorySubscriber struct {
	data *Data
}

func NewInventorySubscriber(data *Data) biz.InventorySubscriber {
	return &InventorySubscriber{data: data}
}

// TODO(3): wrap these subscriber function.
func (s *InventorySubscriber) SubscribeProductCreated(ctx context.Context, handler func(skuIDs []uuid.UUID) error) error {
	topics := []string{biz.KafkaTopicProductCreated}
	consumerHandler := NewConsumerHandler(func(bytes []byte) error {
		var msg ProductCreatedMessage
		if err := json.Unmarshal(bytes, &msg); err != nil {
			return err
		}
		return handler(msg.SkuIDs)
	})
	for {
		err := s.data.GetInventory2ProductConsumer().Consume(ctx, topics, consumerHandler)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func (s *InventorySubscriber) SubscribeOrderCreated(
	ctx context.Context,
	handler func(orderID int64, orderItems []*biz.OrderItem) error,
) error {
	topics := []string{biz.KafkaTopicOrderCreated}
	consumerHandler := NewConsumerHandler(func(bytes []byte) error {
		var msg OrderCreatedMessage
		if err := json.Unmarshal(bytes, &msg); err != nil {
			return err
		}
		var bizOrderItems []*biz.OrderItem
		for _, orderItem := range msg.OrderItems {
			bizOrderItems = append(bizOrderItems, &biz.OrderItem{
				SkuID:    orderItem.SkuID,
				Quantity: orderItem.Quantity,
			})
		}
		return handler(msg.OrderID, bizOrderItems)
	})
	for {
		err := s.data.GetInventory2OrderConsumer().Consume(ctx, topics, consumerHandler)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func (s *InventorySubscriber) SubscribePaymentStatus(
	ctx context.Context,
	handler func(orderID int64, success bool) error,
) error {
	topics := []string{biz.KafkaTopicOrderCreated}
	consumerHandler := NewConsumerHandler(func(bytes []byte) error {
		var msg PaymentStatusMessage
		if err := json.Unmarshal(bytes, &msg); err != nil {
			return err
		}
		return handler(msg.OrderID, msg.Status == PaymentStatusPaid)
	})
	for {
		err := s.data.GetInventory2PaymentConsumer().Consume(ctx, topics, consumerHandler)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}
