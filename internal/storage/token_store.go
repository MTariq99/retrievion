package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
	"github.com/redis/go-redis/v9"
)

func tokenKey(tokenID string) string {
	return fmt.Sprintf("token:%s", tokenID)
}

func userTokensKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%s:tokens", userID)
}

func (r *RedisClient) SaveToken(ctx context.Context, token *models.OpaqueToken) error {
	ttl := time.Until(token.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("token already expired")
	}
	if err := r.Store(ctx, tokenKey(token.TokenID), token, ttl); err != nil {
		return err
	}
	utKey := userTokensKey(token.UserID)
	r.client.SAdd(ctx, utKey, token.TokenID)
	r.client.Expire(ctx, utKey, ttl)
	return nil
}

func (r *RedisClient) GetToken(ctx context.Context, tokenID string) (*models.OpaqueToken, error) {
	raw, err := r.Load(ctx, tokenKey(tokenID))
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, fmt.Errorf("token not found")
	}
	var token models.OpaqueToken
	if err := json.Unmarshal(raw, &token); err != nil {
		return nil, fmt.Errorf("redis GetToken unmarshal: %w", err)
	}
	return &token, nil
}

func (r *RedisClient) RevokeToken(ctx context.Context, tokenID string) error {
	token, err := r.GetToken(ctx, tokenID)
	if err != nil {
		return err
	}
	token.Revoked = true
	return r.SaveToken(ctx, token)
}

func (r *RedisClient) RevokeAllUserTokens(ctx context.Context, userID uuid.UUID) error {
	tokenIDs, err := r.client.SMembers(ctx, userTokensKey(userID)).Result()
	if err != nil {
		return fmt.Errorf("redis RevokeAllUserTokens: %w", err)
	}
	for _, id := range tokenIDs {
		_ = r.RevokeToken(ctx, id)
	}
	return nil
}

func (r *RedisClient) DeleteToken(ctx context.Context, tokenID string) error {
	return r.Delete(ctx, tokenKey(tokenID))
}

func consumedKey(tokenID string) string {
	return fmt.Sprintf("token_consumed:%s", tokenID)
}

// KEYS[1] = token key, KEYS[2] = consumed marker
// Returns: 0 => missing/expired, -1 => already consumed, string => token JSON
var consumeTokenScript = redis.NewScript(`
local data = redis.call('GET', KEYS[1])
if not data then return 0 end

local ttl = redis.call('PTTL', KEYS[1])
if ttl <= 0 then return 0 end

if not redis.call('SET', KEYS[2], ARGV[1], 'NX', 'PX', ttl) then
  return -1
end

return data
`)

func (r *RedisClient) ConsumeToken(ctx context.Context, tokenID string) (*models.OpaqueToken, error) {
	res, err := consumeTokenScript.Run(ctx, r.client, []string{tokenKey(tokenID), consumedKey(tokenID)}, time.Now().UnixMilli()).Result()
	if err != nil {
		return nil, fmt.Errorf("redis ConsumeToken: %w", err)
	}
	switch v := res.(type) {
	case int64:
		if v == -1 {
			return nil, errs.ErrTokenAlreadyConsumed
		}
		return nil, errs.ErrTokenNotFound
	case string:
		var token models.OpaqueToken
		if err := json.Unmarshal([]byte(v), &token); err != nil {
			return nil, fmt.Errorf("redis ConsumeToken unmarshal: %w", err)
		}
		return &token, nil
	default:
		return nil, fmt.Errorf("redis ConsumeToken: unexpected result type %T", res)
	}
}

// ReleaseToken removes the consumed marker (rollback when rotation fails).
func (r *RedisClient) ReleaseToken(ctx context.Context, tokenID string) error {
	return r.Delete(ctx, consumedKey(tokenID))
}

// ConsumedAt returns when the token was consumed, if it was.
func (r *RedisClient) ConsumedAt(ctx context.Context, tokenID string) (time.Time, bool, error) {
	ms, err := r.client.Get(ctx, consumedKey(tokenID)).Int64()
	if errors.Is(err, redis.Nil) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("redis ConsumedAt: %w", err)
	}
	return time.UnixMilli(ms), true, nil
}
