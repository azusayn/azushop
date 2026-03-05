package biz

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
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
	SubscribeProductCreated(ctx context.Context, handler func(skuIDs []uuid.UUID) error) error
	SubscribeOrderCreated(ctx context.Context, handler func(orderID int64, orderItems []*OrderItem) error) error
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
	// mapping from sku_id to quantity.
	// map[uuid.UUID]int64
	Payload []byte              `gorm:"column:payload"`
	Status  InventoryLockStatus `gorm:"column:status"`
}

type InventoryUsecase struct {
	repo       InventoryRepo
	tx         Transaction
	subscriber InventorySubscriber
}

func NewInventoryUsecase(repo InventoryRepo, tx Transaction, sub InventorySubscriber) *InventoryUsecase {
	return &InventoryUsecase{
		repo:       repo,
		tx:         tx,
		subscriber: sub,
	}
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
			return fmt.Errorf("order %q has been procceed", inventoryLock.OrderID)
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

func (uc *InventoryUsecase) HandleProductCreated(ctx context.Context) error {
	return uc.subscriber.SubscribeProductCreated(ctx, func(skuIDs []uuid.UUID) error {
		// TODO(1): retrying topic.
		_, err := uc.repo.BatchCreateInventoris(ctx, skuIDs)
		if err != nil {
			return err
		}
		return err
	})
}

func (uc *InventoryUsecase) HandleOrderCreated(ctx context.Context) error {
	return uc.subscriber.SubscribeOrderCreated(ctx, func(orderID int64, orderItems []*OrderItem) error {
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
	})
}
