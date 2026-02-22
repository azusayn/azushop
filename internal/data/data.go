package data

import (
	"azushop/internal/conf"
	"context"
	"crypto/rsa"
	"database/sql"
	"log/slog"

	"github.com/azusayn/azutils/auth"
	"github.com/google/wire"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(
	NewData,
	NewUserRepo,
)

type Data struct {
	// TODO: wrapped redis client
	// TODO: DDD design.
	postgresClient *sql.DB
	gormClient     *gorm.DB
	PrivateKey     *rsa.PrivateKey
	AppName        string
}

func NewData(c *conf.Data) (*Data, func(), error) {
	key, err := auth.GeneratePrivateKey()
	if err != nil {
		return nil, nil, err
	}

	postgresClient, err := sql.Open(c.Database.Driver, c.Database.Source)
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
		Addr: c.Redis.Addr,
	})
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		postgresClient.Close()
		return nil, nil, err
	}

	cleanup := func() {
		slog.Warn("close postgres connection...")
		if err := postgresClient.Close(); err != nil {
			slog.Warn(err.Error())

		}
		slog.Warn("close redis connection...")
		if err = redisClient.Close(); err != nil {
			slog.Warn(err.Error())
		}
	}

	return &Data{
		PrivateKey:     key,
		postgresClient: postgresClient,
		gormClient:     gormClient,
		AppName:        c.AppName,
	}, cleanup, nil
}
