package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mtariq99/retrievion/models"
	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client *redis.Client
}

func NewRedis(cfg *models.Config) (*RedisClient, error) {
	opts, err := resolveRedisOptions(cfg)
	if err != nil {
		return nil, fmt.Errorf("invalid redis configuration: %w", err)
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	return &RedisClient{client: client}, nil
}

// resolveRedisOptions prefers an explicit REDIS_ADDR (plus REDIS_PASSWORD/
// REDIS_DB) when set, since that's the more specific configuration. When
// REDIS_ADDR is empty — the normal case, since .env only sets REDIS_URL —
// it parses REDIS_URL instead, rather than falling through to go-redis's
// localhost:6379 default, which is wrong inside Docker where Redis is
// reachable only at the "redis" service hostname.
func resolveRedisOptions(cfg *models.Config) (*redis.Options, error) {
	if cfg.Redis.Addr != "" {
		return &redis.Options{
			Addr:         cfg.Redis.Addr,
			Password:     cfg.Redis.Password,
			DB:           cfg.Redis.DB,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			PoolSize:     10,
			MinIdleConns: 5,
		}, nil
	}

	if cfg.Redis.URL == "" {
		return nil, fmt.Errorf("REDIS_ADDR or REDIS_URL must be set")
	}

	opts, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		return nil, fmt.Errorf("parsing REDIS_URL: %w", err)
	}
	opts.DialTimeout = 5 * time.Second
	opts.ReadTimeout = 3 * time.Second
	opts.WriteTimeout = 3 * time.Second
	opts.PoolSize = 10
	opts.MinIdleConns = 5

	return opts, nil
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

func (r *RedisClient) Store(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("redis store marshal [%s]: %w", key, err)
	}
	if err := r.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("redis store set [%s]: %w", key, err)
	}
	return nil
}

func (r *RedisClient) Load(ctx context.Context, key string) ([]byte, error) {
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("redis load get [%s]: %w", key, err)
	}
	return data, nil
}

func (r *RedisClient) Delete(ctx context.Context, keys ...string) error {
	if err := r.client.Del(ctx, keys...).Err(); err != nil && err != redis.Nil {
		return fmt.Errorf("redis delete %v: %w", keys, err)
	}
	return nil
}

// IncrByFloat atomically increments a numeric key by delta and returns
// the NEW total. This MUST be atomic — a read-then-write using
// Load()+Store() would have a race window between two concurrent
// requests both reading the same "before" value (exactly the kind of
// race Retrievion's approval-locking already guards against elsewhere
// with SELECT ... FOR UPDATE; Redis's INCRBYFLOAT is the equivalent
// atomic primitive for a counter instead of a row).
func (r *RedisClient) IncrByFloat(ctx context.Context, key string, delta float64) (float64, error) {
	total, err := r.client.IncrByFloat(ctx, key, delta).Result()
	if err != nil {
		return 0, err
	}
	return total, nil
}

// GetFloat reads a numeric counter, returning 0 (not an error) if the
// key doesn't exist yet — a user's first request of the day has no
// prior spend recorded, and that's a normal state, not a failure.
// This differs deliberately from your Load()'s redis.Nil pass-through:
// budget checks want "no spend yet" to behave as 0, not as an error
// callers have to special-case every time.
func (r *RedisClient) GetFloat(ctx context.Context, key string) (float64, error) {
	val, err := r.client.Get(ctx, key).Float64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("redis getfloat [%s]: %w", key, err)
	}

	return val, nil
}

// Expire sets a TTL on an existing key — used so daily cost counters
// clean themselves up automatically rather than accumulating forever.
func (r *RedisClient) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if err := r.client.Expire(ctx, key, ttl).Err(); err != nil {
		return fmt.Errorf("redis expire [%s]: %w", key, err)
	}
	return nil
}
