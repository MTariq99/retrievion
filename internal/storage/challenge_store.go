package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/redis/go-redis/v9"
)

var ErrChallengeNotFound = errors.New("challenge not found")

func (r *RedisClient) SaveLoginChallenge(ctx context.Context, key string, value any, ttl time.Duration) error {
	return r.Store(ctx, key, value, ttl)
}

// ConsumeLoginChallenge atomically reads and deletes the key (single use).
func (r *RedisClient) ConsumeLoginChallenge(ctx context.Context, key string) ([]byte, error) {
	data, err := r.client.GetDel(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrChallengeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("redis consume challenge: %w", err)
	}
	return data, nil
}
