package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/models"
)

func tokenKey(tokenID string) string {
	return fmt.Sprintf("token:%s", tokenID)
}

func userTokensKey(userID uuid.UUID) string {
	return fmt.Sprintf("user:%d:tokens", userID)
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
