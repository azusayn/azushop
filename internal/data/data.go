package data

import (
	"azushop/internal/conf"
	"context"
	"crypto/rsa"
	"database/sql"
	"log/slog"

	inventorypb "azushop/api/inventory/v1"
	orderpb "azushop/api/order/v1"
	productpb "azushop/api/product/v1"

	"github.com/IBM/sarama"
	"github.com/azusayn/azutils/auth"
	"github.com/google/wire"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"github.com/stripe/stripe-go/v84"
	"go.uber.org/multierr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(
	NewData,
	NewUserRepo,
)

type Data struct {
	// TODO: DDD design.
	postgresClient   *sql.DB
	gormClient       *gorm.DB
	redisClient      *redis.Client
	productService   productpb.ProductServiceClient
	inventoryService inventorypb.InventoryServiceClient
	orderService     orderpb.OrderServiceClient
	kafkaProducer    sarama.SyncProducer
	kafkaConsumer    sarama.Consumer
	privateKey       *rsa.PrivateKey
	stripeSuccessURL string
	appName          string
}

func NewData(c *conf.Data) (*Data, func(), error) {
	stripe.Key = c.GetPayment().GetStripeSecretKey()

	key, err := auth.GeneratePrivateKey()
	if err != nil {
		return nil, nil, err
	}

	// postgres client.
	postgresClient, err := sql.Open(c.GetDatabase().GetDriver(), c.Database.Source)
	if err != nil {
		return nil, nil, err
	}

	pgConfig := postgres.Config{Conn: postgresClient}
	gormClient, err := gorm.Open(postgres.New(pgConfig), &gorm.Config{})
	if err != nil {
		postgresClient.Close()
		return nil, nil, err
	}

	// redis client.
	redisClient := redis.NewClient(&redis.Options{
		Addr: c.GetRedis().GetAddr(),
	})
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		return nil, nil, multierr.Append(err, postgresClient.Close())
	}

	// TODO(1): mtls.
	// grpc service clients.
	productServiceAddr := c.GetService().GetProductServiceAddr()
	productServiceConn, err := grpc.NewClient(productServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		err = multierr.Combine(
			err,
			postgresClient.Close(),
			redisClient.Close(),
		)
		return nil, nil, err
	}

	inventoryServiceAddr := c.GetService().GetInventoryServiceAddr()
	inventoryServiceConn, err := grpc.NewClient(inventoryServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		err = multierr.Combine(
			err,
			postgresClient.Close(),
			redisClient.Close(),
			productServiceConn.Close(),
		)
		return nil, nil, err
	}

	orderServiceAddr := c.GetService().GetOrderServiceAddr()
	orderServiceConn, err := grpc.NewClient(orderServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		err = multierr.Combine(
			err,
			postgresClient.Close(),
			redisClient.Close(),
			productServiceConn.Close(),
			inventoryServiceConn.Close(),
		)
		return nil, nil, err
	}

	// kafka producer & client.
	// TODO(1): async producer.
	brokerAddrs := c.GetKafka().GetBrokerAddrs()
	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Producer.RequiredAcks = sarama.WaitForAll
	kafkaConfig.Producer.Return.Successes = true
	kafkaProducer, err := sarama.NewSyncProducer(brokerAddrs, kafkaConfig)
	if err != nil {
		err = multierr.Combine(
			err,
			postgresClient.Close(),
			redisClient.Close(),
			productServiceConn.Close(),
			inventoryServiceConn.Close(),
			orderServiceConn.Close(),
		)
		return nil, nil, err
	}

	kafkaConsumer, err := sarama.NewConsumer(brokerAddrs, sarama.NewConfig())
	if err != nil {
		err = multierr.Combine(
			err,
			postgresClient.Close(),
			redisClient.Close(),
			productServiceConn.Close(),
			inventoryServiceConn.Close(),
			orderServiceConn.Close(),
			kafkaConsumer.Close(),
		)
		return nil, nil, err
	}

	cleanup := func() {
		err = multierr.Combine(
			err,
			postgresClient.Close(),
			redisClient.Close(),
			productServiceConn.Close(),
			inventoryServiceConn.Close(),
			orderServiceConn.Close(),
			kafkaProducer.Close(),
			kafkaConsumer.Close(),
		)
		if err != nil {
			slog.Warn(err.Error())
		}
	}

	return &Data{
		privateKey:       key,
		postgresClient:   postgresClient,
		gormClient:       gormClient,
		appName:          c.AppName,
		productService:   productpb.NewProductServiceClient(productServiceConn),
		inventoryService: inventorypb.NewInventoryServiceClient(inventoryServiceConn),
		orderService:     orderpb.NewOrderServiceClient(orderServiceConn),
		stripeSuccessURL: c.GetPayment().GetStripeSuccessUrl(),
		kafkaProducer:    kafkaProducer,
		kafkaConsumer:    kafkaConsumer,
	}, cleanup, nil
}

func (d *Data) GetPrivateKey() *rsa.PrivateKey {
	if d == nil {
		return nil
	}
	return d.privateKey
}

func (d *Data) GetAppName() string {
	if d == nil {
		return ""
	}
	return d.appName
}

func (d *Data) GetProductService() productpb.ProductServiceClient {
	if d == nil {
		return nil
	}
	return d.productService
}

func (d *Data) GetIventoryService() inventorypb.InventoryServiceClient {
	if d == nil {
		return nil
	}
	return d.inventoryService
}

func (d *Data) GetOrderService() orderpb.OrderServiceClient {
	if d == nil {
		return nil
	}
	return d.orderService
}

func (d *Data) GetKafkaConsumer() sarama.Consumer {
	if d == nil {
		return nil
	}
	return d.kafkaConsumer
}

func (d *Data) GetKafkaProducer() sarama.SyncProducer {
	if d == nil {
		return nil
	}
	return d.kafkaProducer
}

func (d *Data) GetStripeSuccessUrl() string {
	if d == nil {
		return ""
	}
	return d.stripeSuccessURL
}
