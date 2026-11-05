package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/azusayn/azushop/internal/biz"
	"github.com/azusayn/azushop/internal/pkg/db"
	"github.com/azusayn/azushop/internal/pkg/grpcclient"
	"github.com/azusayn/azushop/internal/pkg/kafka"
	inventoryv1connect "github.com/azusayn/azushop/proto/api/inventory/v1/v1connect"
	"github.com/azusayn/azushop/proto/conf"

	"uuid"

	"github.com/IBM/sarama"
	"github.com/google/wire"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/propagation"
	"gorm.io/gorm"
)

var InventoryDataProviderSet = wire.NewSet(
	db.NewClient,
	NewInventoryRepo,
	NewInventorySubscriber,
)

type InventoryRepo struct {
	db *db.Client
}

func NewInventoryRepo(db *db.Client) biz.InventoryRepo {
	return &InventoryRepo{db: db}
}

func NewInventoryClient(config *conf.Data, propagator propagation.TextMapPropagator) (inventoryv1connect.InventoryServiceClient, error) {
	opts, err := grpcclient.NewAuthenticatedClientOptions(propagator)
	if err != nil {
		return nil, err
	}
	return grpcclient.NewServiceClient(
		inventoryv1connect.NewInventoryServiceClient,
		config.GetServiceAddr().GetInventory(),
		opts,
	), nil
}

// updates the same set of fields (defined by paths) for each inventory.
func (repo *InventoryRepo) UpdateInventories(ctx context.Context, inventories []*biz.Inventory, paths []string) error {
	for _, inv := range inventories {
		if err := repo.db.GetTransaction(ctx).WithContext(ctx).Model(inv).Select(paths).Updates(inv).Error; err != nil {
			return err
		}
	}
	return nil
}

func (repo *InventoryRepo) BatchGetInventories(ctx context.Context, skuIDs []uuid.UUID) ([]*biz.Inventory, error) {
	if len(skuIDs) == 0 {
		return nil, errors.New("empty skuIDs")
	}
	var inventories []*biz.Inventory
	err := repo.db.GetTransaction(ctx).WithContext(ctx).Where("sku_id IN ?", skuIDs).Find(&inventories).Error
	if err != nil {
		return nil, err
	}
	return inventories, nil
}

func (repo *InventoryRepo) UpdateDeltaQuantity(ctx context.Context, inventories []*biz.Inventory, paths []string) error {
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
		if err := repo.db.GetTransaction(ctx).
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
	var inventoryLock biz.InventoryLock
	if err := repo.db.GetTransaction(ctx).WithContext(ctx).Where("order_id = ?", orderID).Find(&inventoryLock).Error; err != nil {
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
	payload, err := json.Marshal(orderItems)
	if err != nil {
		return err
	}
	lock := &biz.InventoryLock{
		OrderID: orderID,
		Payload: payload,
		Status:  status,
	}
	return repo.db.GetTransaction(ctx).WithContext(ctx).Table("inventory_locks").Create(lock).Error
}

func (repo *InventoryRepo) UpdateInventoryLock(ctx context.Context, inventoryLocks []*biz.InventoryLock, paths []string) error {
	for _, invLock := range inventoryLocks {
		if err := repo.db.GetTransaction(ctx).
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
	var inventories []*biz.Inventory
	for _, skuID := range skuIDs {
		inventories = append(inventories, &biz.Inventory{
			SkuID:            skuID,
			StockQuantity:    0,
			ReservedQuantity: 0,
		})
	}
	if err := repo.db.GetTransaction(ctx).WithContext(ctx).Create(&inventories).Error; err != nil {
		return nil, err
	}
	return inventories, nil
}

type InventorySubscriber struct {
	handlers      map[kafka.TopicType]kafka.HandlerFunc
	consumerGroup sarama.ConsumerGroup
}

func NewInventorySubscriber(config *conf.Data) (biz.InventorySubscriber, error) {
	brokerAddrs := config.GetKafka().GetBrokerAddrs()
	consumerGroup, err := kafka.NewConsumerGroup(brokerAddrs, config.GetServiceName())
	if err != nil {
		return nil, err
	}
	return &InventorySubscriber{
		handlers:      make(map[kafka.TopicType]kafka.HandlerFunc),
		consumerGroup: consumerGroup,
	}, nil
}

func (s *InventorySubscriber) Subscribe(ctx context.Context) error {
	consumerHandler := kafka.NewConsumerHandler(s.handlers)
	topics := make([]string, 0, len(s.handlers))
	for topic := range s.handlers {
		topics = append(topics, string(topic))
	}
	for {
		err := s.consumerGroup.Consume(ctx, topics, consumerHandler)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func (s *InventorySubscriber) RegisterHandler(topic kafka.TopicType, handler func(context.Context, []byte) error) {
	s.handlers[topic] = handler
}
