package data

import (
	"azushop/internal/conf"
	"context"
	"crypto/rsa"
	"database/sql"
	"log/slog"

	productpb "azushop/api/product/v1"

	"github.com/azusayn/azutils/auth"
	"github.com/google/wire"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
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
	postgresClient *sql.DB
	gormClient     *gorm.DB
	redisClient    *redis.Client
	productService productpb.ProductServiceClient
	privateKey     *rsa.PrivateKey
	appName        string
}

func NewData(c *conf.Data) (*Data, func(), error) {
	key, err := auth.GeneratePrivateKey()
	if err != nil {
		return nil, nil, err
	}

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

	redisClient := redis.NewClient(&redis.Options{
		Addr: c.GetRedis().GetAddr(),
	})
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		postgresClient.Close()
		return nil, nil, err
	}

	// TODO(1): tls.
	productServiceAddr := c.GetService().GetProductServiceAddr()
	productServiceConn, err := grpc.NewClient(productServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		if err := postgresClient.Close(); err != nil {
			slog.Warn(err.Error())
		}
		if err = redisClient.Close(); err != nil {
			slog.Warn(err.Error())
		}
		return nil, nil, err
	}
	cleanup := func() {
		slog.Warn("close postgres connection...")
		if err := postgresClient.Close(); err != nil {
			slog.Warn(err.Error())
		}
		slog.Warn("close redis connection...")
		if err := redisClient.Close(); err != nil {
			slog.Warn(err.Error())
		}
		slog.Warn("close ProductService connection...")
		if err := productServiceConn.Close(); err != nil {
			slog.Warn(err.Error())
		}
	}

	return &Data{
		privateKey:     key,
		postgresClient: postgresClient,
		gormClient:     gormClient,
		appName:        c.AppName,
		productService: productpb.NewProductServiceClient(productServiceConn),
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
