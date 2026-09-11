package data

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/azusayn/azushop/proto/conf"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

const (
	// expiration jitter ratio range to reduce cache avalanches.
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

	if err := redisotel.InstrumentTracing(client); err != nil {
		return nil, fmt.Errorf("failed to instrument redis: %w", err)
	}

	return &Redis{Client: client}, nil
}

func GetCache[T any](ctx context.Context, r *Redis, key string) (T, bool) {
	var zero T

	bytes, err := r.Client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			slog.DebugContext(ctx, "cache miss", slog.String("key", key))
			return zero, false
		}
		slog.WarnContext(ctx, "failed to get cache", slog.Any("err", err))
		return zero, false
	}

	var val T
	if err := json.Unmarshal(bytes, &val); err != nil {
		slog.WarnContext(ctx, "failed to unmarshal cache", slog.Any("err", err))
		return zero, false
	}

	return val, true
}

func SetCache(ctx context.Context, r *Redis, key string, val any, expiration time.Duration) {
	bytes, err := json.Marshal(val)
	if err != nil {
		slog.WarnContext(ctx, "failed to marshal value", slog.Any("err", err))
		return
	}

	ratio := cacheJitterMin + (cacheJitterMax-cacheJitterMin)*rand.Float64()
	jitter := float64(expiration) * ratio
	if err := r.Client.Set(ctx, key, bytes, expiration+time.Duration(jitter)).Err(); err != nil {
		slog.WarnContext(ctx, "failed to set cache", slog.Any("err", err))
	}
}

func DelCache(ctx context.Context, r *Redis, keys ...string) {
	client := r.Client
	if err := client.Del(ctx, keys...).Err(); err != nil {
		slog.WarnContext(ctx, "failed to delete cache", slog.Any("err", err))
	}
}

func SetCacheSAdd(ctx context.Context, r *Redis, key string, members ...any) {
	client := r.Client
	if err := client.SAdd(ctx, key, members).Err(); err != nil {
		slog.WarnContext(ctx, "failed to sadd cache", slog.Any("err", err))
	}
}

// GetCacheSMembers returns true if any keys are found.
func GetCacheSMembers(ctx context.Context, r *Redis, key string) ([]string, bool) {
	strs, err := r.Client.SMembers(ctx, key).Result()
	if err != nil {
		slog.WarnContext(ctx, "failed to get cache smembers", slog.Any("err", err))
		return nil, false
	}
	if len(strs) == 0 {
		return nil, false
	}
	return strs, true
}
