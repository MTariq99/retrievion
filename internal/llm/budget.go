package llm

import (
	"context"
	"fmt"
	"time"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/storage"
)

type Limits struct {
	MaxCostUSDPerDay float64
	MaxTokensPerDay  int
}

func costKey(userID string) string {
	return fmt.Sprintf("cost:%s:%s", userID, time.Now().UTC().Format("2006-01-02"))
}

func tokenKey(userID string) string {
	return fmt.Sprintf("tokens:%s:%s", userID, time.Now().UTC().Format("2006-01-02"))
}

func (c *Client) CheckBudget(ctx context.Context, userID string, limits Limits) error {
	spent, err := c.redisClient.GetFloat(ctx, costKey(userID))
	if err != nil {
		return fmt.Errorf("budget check unavailable, failing closed: %w", err)
	}

	if spent >= limits.MaxCostUSDPerDay {
		return fmt.Errorf("daily cost budget exceeded: $%.4f / $%.2f", spent, limits.MaxCostUSDPerDay)
	}
	return nil
}

func (c *Client) RecordSpend(ctx context.Context, redis *storage.RedisClient, userID string, costUSD float64, tokens int) error {
	cKey := costKey(userID)
	tKey := tokenKey(userID)

	newTotal, err := c.redisClient.IncrByFloat(ctx, cKey, costUSD)
	if err != nil {
		return fmt.Errorf("failed to record spend: %w", err)
	}

	if newTotal == costUSD {
		if err := redis.Expire(ctx, cKey, consts.CostSpendTTL); err != nil {
			return fmt.Errorf("failed to set cost key TTL: %w", err)
		}
	}

	tokenTotal, err := redis.IncrByFloat(ctx, tKey, float64(tokens))
	if err != nil {
		return fmt.Errorf("failed to record token usage: %w", err)
	}
	if tokenTotal == float64(tokens) {
		if err := redis.Expire(ctx, tKey, 25*time.Hour); err != nil {
			return fmt.Errorf("failed to set token key TTL: %w", err)
		}
	}

	return nil
}

func (c *Client) CheckTokenBudget(ctx context.Context, userID string, limits Limits) error {
	used, err := c.redisClient.GetFloat(ctx, tokenKey(userID))
	if err != nil {
		return fmt.Errorf("token budget check unavailable, failing closed: %w", err)
	}
	if int(used) >= limits.MaxTokensPerDay {
		return fmt.Errorf("daily token budget exceeded: %d / %d", int(used), limits.MaxTokensPerDay)
	}
	return nil
}
