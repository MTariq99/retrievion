package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mtariq99/retrievion/internal/consts"
)

func sessionKey(userID int64, sessionID string) string {
	return fmt.Sprintf("goal_setup:%d:%s", userID, sessionID)
}

func (r *RedisClient) SetSession(ctx context.Context, userID int64, sessionID string, value any) error {
	return r.Store(ctx, sessionKey(userID, sessionID), value, consts.SessionTTL)
}

func (r *RedisClient) GetSession(ctx context.Context, userID int64, sessionID string, dst any) (bool, error) {
	raw, err := r.Load(ctx, sessionKey(userID, sessionID))
	if err != nil {
		return false, err
	}
	if raw == nil {
		return false, nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return false, fmt.Errorf("redis GetSession unmarshal: %w", err)
	}
	return true, nil
}

func (r *RedisClient) DeleteSession(ctx context.Context, userID int64, sessionID string) error {
	return r.Delete(ctx, sessionKey(userID, sessionID))
}
