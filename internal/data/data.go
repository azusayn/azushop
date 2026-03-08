package data

import (
	"azushop/internal/conf"
	"context"
	"crypto/rsa"
	"database/sql"
	"fmt"
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

const (
	ServiceNameOrder     = "service.order"
	ServiceNameInventory = "service.inventory"
	ServiceNameProduct   = "service.product"
	ServiceNamePayment   = "service.payment"
)

var ProviderSet = wire.NewSet(
	NewData,
	NewTransaction,
	NewPaymentPublisher,
	NewOrderSubscriber,
	NewOrderPublisher,
	NewProductPublisher,
	NewInventorySubscriber,
	NewDelayMsgRelaySubscriber,
	NewDelayRelayPublisher,
	NewUserRepo,
	NewProductRepo,
	NewInventoryRepo,
	NewOrderRepo,
	NewPaymentRepo,
)

type Data struct {
	// TODO(3): DDD design.
	postgresClient *sql.DB
	gormClient     *gorm.DB
	redisClient    *redis.Client
	// mapping from service name to client conn.
	serviceConns  map[string]*ServiceConn
	kafkaProducer sarama.SyncProducer
	// mapping from service name to consumer group.
	kafkaConsumers   map[string]sarama.ConsumerGroup
	privateKey       *rsa.PrivateKey
	stripeSuccessURL string
	appName          string
}

type ServiceConn struct {
	Addr string
	conn *grpc.ClientConn
}

func NewData(c *conf.Data) (*Data, func(), error) {
	stripe.Key = c.GetPayment().GetStripeSecretKey()

	key, err := auth.GeneratePrivateKey()
	if err != nil {
		return nil, nil, err
	}

	// postgres client.
	postgresClient, err := sql.Open(c.GetDatabase().GetDriver(), c.GetDatabase().GetSource())
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
	// grpc service client conns
	serviceConns := map[string]*ServiceConn{
		ServiceNameOrder:     {Addr: c.GetServiceAddr().GetOrder()},
		ServiceNameInventory: {Addr: c.GetServiceAddr().GetInventory()},
		ServiceNameProduct:   {Addr: c.GetServiceAddr().GetProduct()},
	}
	for name, serviceConn := range serviceConns {
		conn, err := grpc.NewClient(
			serviceConn.Addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			err = multierr.Combine(
				err,
				postgresClient.Close(),
				redisClient.Close(),
			)
			for _, serviceConn := range serviceConns {
				err = multierr.Append(err, serviceConn.conn.Close())
			}
			return nil, nil, err
		}
		serviceConns[name].conn = conn
	}

	// kafka producer & client.
	// TODO(1): async producer.
	brokerAddrs := c.GetKafka().GetBrokerAddrs()
	slog.Info(fmt.Sprintf("%v", brokerAddrs))
	if len(brokerAddrs) == 0 {
		panic("wtf bro...")
	}
	kafkaProducer, err := NewSyncProducer(brokerAddrs)
	if err != nil {
		err = multierr.Combine(
			err,
			postgresClient.Close(),
			redisClient.Close(),
		)
		for _, serviceConn := range serviceConns {
			err = multierr.Append(err, serviceConn.conn.Close())
		}
		return nil, nil, err
	}
	kafkaConsumers := map[string]sarama.ConsumerGroup{
		"order2payment":       nil,
		"order2order":         nil,
		"inventory2product":   nil,
		"inventory2order":     nil,
		"inventory2payment":   nil,
		"delayMsgRelay2Order": nil,
	}
	for name := range kafkaConsumers {
		consumer, err := NewConsumerGroup(brokerAddrs, name)
		if err != nil {
			err = multierr.Combine(
				err,
				postgresClient.Close(),
				redisClient.Close(),
				kafkaProducer.Close(),
			)
			for _, serviceConn := range serviceConns {
				err = multierr.Append(err, serviceConn.conn.Close())
			}
			for _, consumer := range kafkaConsumers {
				err = multierr.Append(err, consumer.Close())
			}
			return nil, nil, err
		}
		kafkaConsumers[name] = consumer
	}

	cleanup := func() {
		err = multierr.Combine(
			err,
			postgresClient.Close(),
			redisClient.Close(),
			kafkaProducer.Close(),
		)
		for _, serviceConn := range serviceConns {
			err = multierr.Append(err, serviceConn.conn.Close())
		}
		for _, consumer := range kafkaConsumers {
			err = multierr.Append(err, consumer.Close())
		}
		if err != nil {
			slog.Warn(err.Error())
		}
	}

	return &Data{
		privateKey:       key,
		postgresClient:   postgresClient,
		redisClient:      redisClient,
		gormClient:       gormClient,
		appName:          c.AppName,
		serviceConns:     serviceConns,
		kafkaProducer:    kafkaProducer,
		kafkaConsumers:   kafkaConsumers,
		stripeSuccessURL: c.GetPayment().GetStripeSuccessUrl(),
	}, cleanup, nil
}

func (d *Data) GetPrivateKey() *rsa.PrivateKey {
	return d.privateKey
}

func (d *Data) GetAppName() string {
	return d.appName
}

func (d *Data) GetProductService() productpb.ProductServiceClient {
	return productpb.NewProductServiceClient(d.serviceConns[ServiceNameProduct].conn)
}

func (d *Data) GetIventoryService() inventorypb.InventoryServiceClient {
	return inventorypb.NewInventoryServiceClient(d.serviceConns[ServiceNameInventory].conn)
}

func (d *Data) GetOrderService() orderpb.OrderServiceClient {
	return orderpb.NewOrderServiceClient(d.serviceConns[ServiceNameOrder].conn)
}

func (d *Data) GetOrder2PaymentConsumer() sarama.ConsumerGroup {
	return d.kafkaConsumers["order2payment"]
}

func (d *Data) GetOrder2OrderConsumer() sarama.ConsumerGroup {
	return d.kafkaConsumers["order2order"]
}

func (d *Data) GetInventory2ProductConsumer() sarama.ConsumerGroup {
	return d.kafkaConsumers["inventory2product"]
}

func (d *Data) GetInventory2PaymentConsumer() sarama.ConsumerGroup {
	return d.kafkaConsumers["inventory2payment"]
}

func (d *Data) GetInventory2OrderConsumer() sarama.ConsumerGroup {
	return d.kafkaConsumers["inventory2order"]
}

func (d *Data) GetDelayMsgRelay2Order() sarama.ConsumerGroup {
	return d.kafkaConsumers["delayMsgRelay2Order"]
}

func (d *Data) GetKafkaProducer() sarama.SyncProducer {
	return d.kafkaProducer
}

func (d *Data) GetStripeSuccessUrl() string {
	return d.stripeSuccessURL
}
