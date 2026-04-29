package data

import (
	"azushop/internal/conf"
	"context"
	"encoding/json"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

const (
	// jitter percentage range for cache expiration.
	cacheJitterMin float64 = 0.05
	cacheJitterMax float64 = 0.10
)

type Redis struct {
	Client *redis.Client
}

func NewRedis(config *conf.Data) (*Redis, error) {
	client := redis.NewClient(&redis.Options{Addr: config.GetRedis().GetAddr()})
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, errors.Wrap(err, "failed to init redis client")
	}
	return &Redis{Client: client}, nil
}

func GetCache[T any](ctx context.Context, r *Redis, key string) (T, bool) {
	var zero T
	val, err := func() (T, error) {
		client := r.Client
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

func SetCache(ctx context.Context, r *Redis, key string, val any, expiration time.Duration) {
	err := func() error {
		client := r.Client
		bytes, err := json.Marshal(val)
		if err != nil {
			return err
		}
		ratio := cacheJitterMin + (cacheJitterMax-cacheJitterMin)*rand.Float64()
		jitter := float64(expiration) * ratio
		return client.Set(ctx, key, bytes, expiration+time.Duration(jitter)).Err()
	}()
	if err != nil {
		slog.Warn(err.Error())
	}
}

func DelCache(ctx context.Context, r *Redis, keys ...string) {
	client := r.Client
	if err := client.Del(ctx, keys...).Err(); err != nil {
		slog.Warn(err.Error())
	}
}

func SetCacheSAdd(ctx context.Context, r *Redis, key string, members ...any) {
	client := r.Client
	if err := client.SAdd(ctx, key, members).Err(); err != nil {
		slog.Warn(err.Error())
	}
}

// returns true if any keys are found.
func GetCacheSMembers(ctx context.Context, r *Redis, key string) ([]string, bool) {
	strs, err := r.Client.SMembers(ctx, key).Result()
	if err != nil {
		slog.Warn(err.Error())
		return nil, false
	}
	if len(strs) == 0 {
		return nil, false
	}
	return strs, true
}
