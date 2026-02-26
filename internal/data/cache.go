package data

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// jitter percentage range for cache expiration.
	cacheJitterMin float64 = 0.05
	cacheJitterMax float64 = 0.10
)

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

func SetCache(ctx context.Context, data *Data, key string, val any, expiration time.Duration) {
	err := func() error {
		client := data.redisClient
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

func DelCache(ctx context.Context, data *Data, keys ...string) {
	client := data.redisClient
	if err := client.Del(ctx, keys...).Err(); err != nil {
		slog.Warn(err.Error())
	}
}

func SetCacheSAdd(ctx context.Context, data *Data, key string, members ...any) {
	client := data.redisClient
	if err := client.SAdd(ctx, key, members).Err(); err != nil {
		slog.Warn(err.Error())
	}
}

func GetCacheSMembers(ctx context.Context, data *Data, key string) ([]string, bool) {
	strs, err := data.redisClient.SMembers(ctx, key).Result()
	if err != nil {
		slog.Warn(err.Error())
		return nil, false
	}
	return strs, true
}
