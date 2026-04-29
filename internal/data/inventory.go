package data

import (
	"azushop/internal/biz"
	"azushop/internal/conf"
	"context"
	"encoding/json"
	"fmt"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"github.com/google/wire"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

var InventoryDataProviderSet = wire.NewSet(
	NewPostgres,
	NewTransactionV2,
	NewInventoryRepo,
	NewInventorySubscriber,
)

type InventoryRepo struct {
	postgres *Postgres
}

func NewInventoryRepo(postgres *Postgres) biz.InventoryRepo {
	return &InventoryRepo{postgres: postgres}
}

// updates the same set of fields (defined by paths) for each inventory.
func (repo *InventoryRepo) UpdateInventories(ctx context.Context, inventories []*biz.Inventory, paths []string) error {
	client := GetTransaction(ctx)
	if client == nil {
		client = repo.postgres.GormClient
	}
	for _, inv := range inventories {
		if err := client.WithContext(ctx).Model(inv).Select(paths).Updates(inv).Error; err != nil {
			return err
		}
	}
	return nil
}

func (repo *InventoryRepo) BatchGetInventories(ctx context.Context, skuIDs []uuid.UUID) ([]*biz.Inventory, error) {
	if len(skuIDs) == 0 {
		return nil, errors.New("empty skuIDs")
	}
	gormClient := repo.postgres.GormClient
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
		gormClient = repo.postgres.GormClient
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
		gormClient = repo.postgres.GormClient
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
		gormClient = repo.postgres.GormClient
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
		client = repo.postgres.GormClient
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
		client = repo.postgres.GormClient
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
	orderCreatedSub   sarama.ConsumerGroup
	productCreatedSub sarama.ConsumerGroup
	paymentStatusSub  sarama.ConsumerGroup
}

func NewInventorySubscriber(config *conf.Data) (biz.InventorySubscriber, error) {
	brokerAddrs := config.GetKafka().GetBrokerAddrs()
	orderCreatedGroupID := "inventory.order.created"
	orderCreatedSub, err := NewConsumerGroup(brokerAddrs, orderCreatedGroupID)
	if err != nil {
		return nil, err
	}
	productCreatedGroupID := "inventory.product.created"
	productCreatedSub, err := NewConsumerGroup(brokerAddrs, productCreatedGroupID)
	if err != nil {
		return nil, err
	}
	paymentStatusGroupID := "inventory.payment.status"
	paymentStatusSub, err := NewConsumerGroup(brokerAddrs, paymentStatusGroupID)
	if err != nil {
		return nil, err
	}
	return &InventorySubscriber{
		orderCreatedSub:   orderCreatedSub,
		productCreatedSub: productCreatedSub,
		paymentStatusSub:  paymentStatusSub,
	}, nil
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
		err := s.productCreatedSub.Consume(ctx, topics, consumerHandler)
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
		err := s.orderCreatedSub.Consume(ctx, topics, consumerHandler)
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
		err := s.paymentStatusSub.Consume(ctx, topics, consumerHandler)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}
