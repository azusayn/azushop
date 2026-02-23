package data

import (
	"azushop/internal/conf"
	"context"
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

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
	// TODO: DDD design.
	postgresClient *sql.DB
	gormClient     *gorm.DB
	redisClient    *redis.Client
	privateKey     *rsa.PrivateKey
	appName        string
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
		privateKey:     key,
		postgresClient: postgresClient,
		gormClient:     gormClient,
		appName:        c.AppName,
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

func GetCache[T any](ctx context.Context, data *Data, key string) (T, bool) {
	var zero T
	val, err := func() (T, error) {
		client := data.redisClient
		bytes, err := client.Get(ctx, key).Bytes()
		if err != nil {
			return zero, err
		}
		var val T
		if err := json.Unmarshal(bytes, &val); err != nil {
			return zero, err
		}
		return val, nil
	}()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			slog.Warn(err.Error())
		}
		return zero, false
	}
	return val, true
}

func SetCache[T any](ctx context.Context, data *Data, key string, val T, expiration time.Duration) {
	err := func() error {
		client := data.redisClient
		bytes, err := json.Marshal(val)
		if err != nil {
			return err
		}
		return client.Set(ctx, key, bytes, expiration).Err()
	}()
	if err != nil {
		slog.Warn(err.Error())
	}
}
