package biz

import (
	"context"
	"encoding/json"
	"fmt"

	"uuid"

	"github.com/azusayn/azushop/internal/pkg/db"
	"github.com/azusayn/azushop/internal/pkg/kafka"
	"github.com/azusayn/azushop/internal/pkg/outbox"
	"github.com/azusayn/azushop/proto/conf"
	"github.com/google/wire"
)

var InventoryBizProviderSet = wire.NewSet(
	NewInventoryUsecase,
)

type InventoryRepo interface {
	// 'inventory' table.
	// set the values directly.
	UpdateInventories(ctx context.Context, inventories []*Inventory, paths []string) error
	// use delta to update the stock quantity.
	UpdateDeltaQuantity(ctx context.Context, inventories []*Inventory, paths []string) error
	BatchGetInventories(ctx context.Context, skuIDs []uuid.UUID) ([]*Inventory, error)
	BatchCreateInventoris(ctx context.Context, skuIDs []uuid.UUID) ([]*Inventory, error)

	// 'inventory_lock' table.
	GetInventoryLock(ctx context.Context, orderID int64) (*InventoryLock, error)
	CreateInventoryLock(ctx context.Context, orderID int64, orderItems []*OrderItem, status InventoryLockStatus) error
	UpdateInventoryLock(ctx context.Context, inventoryLocks []*InventoryLock, paths []string) error
}

type InventorySubscriber interface {
	RegisterHandler(topic kafka.TopicType, handler func(context.Context, []byte) error)
	Subscribe(ctx context.Context) error
}

type Inventory struct {
	SkuID            uuid.UUID `gorm:"column:sku_id"`
	StockQuantity    int64     `gorm:"column:stock_quantity"`
	ReservedQuantity int64     `gorm:"column:reserved_quantity"`
}

type InventoryLockStatus string

const (
	InventoryLockStatusLocked    InventoryLockStatus = "locked"
	InventoryLockStatusConfirmed InventoryLockStatus = "confirmed"
	InventoryLockStatusReleased  InventoryLockStatus = "released"
)

type InventoryLock struct {
	OrderID int64 `gorm:"column:order_id"`
	// []*biz.OrderItem
	Payload []byte              `gorm:"column:payload"`
	Status  InventoryLockStatus `gorm:"column:status"`
}

type ProductCreatedMessage struct {
	SkuIDs []uuid.UUID
}

type InventoryUsecase struct {
	repo       InventoryRepo
	tx         *db.Transaction
	subscriber InventorySubscriber
	outbox     *outbox.Outbox
}

func NewInventoryUsecase(
	repo InventoryRepo,
	tx *db.Transaction,
	sub InventorySubscriber,
	config *conf.Data,
) *InventoryUsecase {
	uc := &InventoryUsecase{
		repo:       repo,
		tx:         tx,
		subscriber: sub,
	}
	uc.outbox = outbox.NewOutbox(config.GetServiceName(), uc.buildDispatchFunc(), tx)
	return uc
}

func (uc *InventoryUsecase) AdjustStock(
	ctx context.Context,
	skuID uuid.UUID,
	stockQuantity int64,
	role UserRole,
) error {
	inventories := []*Inventory{{
		SkuID:         skuID,
		StockQuantity: stockQuantity,
	}}
	paths := []string{"stock_quantity"}

	err := uc.tx.Transaction(ctx, func(ctx context.Context) error {
		return uc.repo.UpdateInventories(ctx, inventories, paths)
	})
	return err
}

func (uc *InventoryUsecase) BatchGetInventories(ctx context.Context, skuIDs []uuid.UUID) ([]*Inventory, error) {
	return uc.repo.BatchGetInventories(ctx, skuIDs)
}

func (uc *InventoryUsecase) ReserveStock(ctx context.Context, orderID int64, orderItems []*OrderItem) error {
	var inventoryDeltas []*Inventory
	for _, orderItem := range orderItems {
		inventoryDeltas = append(inventoryDeltas, &Inventory{
			SkuID:            orderItem.SkuID,
			ReservedQuantity: orderItem.Quantity,
		})
	}
	paths := []string{"reserved_quantity"}
	return uc.tx.Transaction(ctx, func(ctx context.Context) error {
		if err := uc.repo.UpdateDeltaQuantity(ctx, inventoryDeltas, paths); err != nil {
			return err
		}
		return uc.repo.CreateInventoryLock(ctx, orderID, orderItems, InventoryLockStatusLocked)
	})
}

func (uc *InventoryUsecase) ReleaseStock(ctx context.Context, orderID int64) error {
	return uc.tx.Transaction(ctx, func(ctx context.Context) error {
		inventoryLock, err := uc.repo.GetInventoryLock(ctx, orderID)
		if err != nil {
			return err
		}
		var orderItems []*OrderItem
		if err := json.Unmarshal(inventoryLock.Payload, &orderItems); err != nil {
			return err
		}
		var inventoryDeltas []*Inventory
		for _, orderItem := range orderItems {
			inventoryDeltas = append(inventoryDeltas, &Inventory{
				SkuID:            orderItem.SkuID,
				ReservedQuantity: -orderItem.Quantity,
			})
		}
		if err := uc.repo.UpdateInventories(ctx, inventoryDeltas, []string{"reserved_quantity"}); err != nil {
			return err
		}
		inventoryLocks := []*InventoryLock{{
			OrderID: orderID,
			Status:  InventoryLockStatusReleased,
		}}
		return uc.repo.UpdateInventoryLock(ctx, inventoryLocks, []string{"status"})
	})
}

func (uc *InventoryUsecase) DeductStock(ctx context.Context, orderID int64) error {
	return uc.tx.Transaction(ctx, func(ctx context.Context) error {
		inventoryLock, err := uc.repo.GetInventoryLock(ctx, orderID)
		if err != nil {
			return err
		}
		if inventoryLock.Status != InventoryLockStatusLocked {
			return fmt.Errorf("order %d has been procceed", inventoryLock.OrderID)
		}
		var orderItems []*OrderItem
		if err := json.Unmarshal(inventoryLock.Payload, &orderItems); err != nil {
			return err
		}
		var inventoryDeltas []*Inventory
		for _, orderItem := range orderItems {
			inventoryDeltas = append(inventoryDeltas, &Inventory{
				SkuID:            orderItem.SkuID,
				StockQuantity:    -orderItem.Quantity,
				ReservedQuantity: -orderItem.Quantity,
			})
		}
		paths := []string{"stock_quantity", "reserved_quantity"}
		if err := uc.repo.UpdateInventories(ctx, inventoryDeltas, paths); err != nil {
			return err
		}
		inventoryLocks := []*InventoryLock{{
			OrderID: orderID,
			Status:  InventoryLockStatusConfirmed,
		}}
		return uc.repo.UpdateInventoryLock(ctx, inventoryLocks, []string{"status"})
	})
}

func (uc *InventoryUsecase) HandleKafkaMessages(ctx context.Context) error {
	uc.subscriber.RegisterHandler(KafkaTopicProductCreated, uc.handleProductCreated)
	uc.subscriber.RegisterHandler(KafkaTopicOrderCreated, uc.handleOrderCreated)
	uc.subscriber.RegisterHandler(KafkaTopicPaymentStatus, uc.handlePaymentStatus)
	return uc.subscriber.Subscribe(ctx)
}

func (uc *InventoryUsecase) HandleOutboxMessages(ctx context.Context) error {
	return uc.outbox.Run(ctx)
}

func (uc *InventoryUsecase) handleProductCreated(ctx context.Context, bytes []byte) error {
	var msg ProductCreatedMessage
	if err := json.Unmarshal(bytes, &msg); err != nil {
		return err
	}
	return uc.saveOutbox(ctx, outboxEventInventoryProductCreated, bytes)
}

func (uc *InventoryUsecase) handleOrderCreated(ctx context.Context, bytes []byte) error {
	var msg OrderCreatedMessage
	if err := json.Unmarshal(bytes, &msg); err != nil {
		return err
	}
	return uc.saveOutbox(ctx, outboxEventInventoryOrderCreated, bytes)
}

func (uc *InventoryUsecase) handlePaymentStatus(ctx context.Context, bytes []byte) error {
	var msg PaymentStatusMessage
	if err := json.Unmarshal(bytes, &msg); err != nil {
		return err
	}
	if msg.Status != PaymentStatusPaid {
		return nil
	}
	return uc.saveOutbox(ctx, outboxEventInventoryPaymentStatus, bytes)
}

func (uc *InventoryUsecase) saveOutbox(ctx context.Context, eventType OutboxEventType, payload []byte) error {
	return uc.tx.Transaction(ctx, func(ctx context.Context) error {
		return uc.outbox.Save(ctx, string(eventType), payload)
	})
}

func (uc *InventoryUsecase) dispatchOutboxMessage(ctx context.Context, message *outbox.OutboxMessage) error {
	switch OutboxEventType(message.EventType) {
	case outboxEventInventoryProductCreated:
		var msg ProductCreatedMessage
		if err := json.Unmarshal(message.Payload, &msg); err != nil {
			return err
		}
		return uc.tx.Transaction(ctx, func(ctx context.Context) error {
			_, err := uc.repo.BatchCreateInventoris(ctx, msg.SkuIDs)
			return err
		})

	case outboxEventInventoryOrderCreated:
		var msg OrderCreatedMessage
		if err := json.Unmarshal(message.Payload, &msg); err != nil {
			return err
		}
		var orderItems []*OrderItem
		for _, orderItem := range msg.OrderItems {
			orderItems = append(orderItems, &OrderItem{
				SkuID:    orderItem.SkuID,
				Quantity: orderItem.Quantity,
			})
		}
		return uc.ReserveStock(ctx, msg.OrderID, orderItems)

	case outboxEventInventoryPaymentStatus:
		var msg PaymentStatusMessage
		if err := json.Unmarshal(message.Payload, &msg); err != nil {
			return err
		}
		if msg.Status != PaymentStatusPaid {
			return nil
		}
		return uc.DeductStock(ctx, msg.OrderID)

	default:
	}
	return fmt.Errorf("unknown event type %q", message.EventType)
}

func (uc *InventoryUsecase) buildDispatchFunc() outbox.DispatchFunc {
	return func(ctx context.Context, message *outbox.OutboxMessage) error {
		return uc.dispatchOutboxMessage(ctx, message)
	}
}
