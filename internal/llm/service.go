package llm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/mtariq99/retrievion/internal/consts"

	"github.com/mtariq99/retrievion/models"
	"go.uber.org/zap"
)

type PricePerMillionTokens struct {
	Input  float64
	Output float64
}

type Service struct {
	provider Provider
	config   *models.Config
	pricing  PricePerMillionTokens
	logger   *zap.Logger
}

func NewService(provider Provider, cfg *models.Config, pricing PricePerMillionTokens, logger *zap.Logger) *Service {
	return &Service{provider: provider, config: cfg, pricing: pricing, logger: logger}
}

func (s *Service) Generate(ctx context.Context, prompt string) (models.GenerateResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, consts.LLMDefaultCallTimeout)
	defer cancel()

	req := models.GenerateRequest{
		Prompt:      prompt,
		MaxTokens:   s.config.LLM.MaxTokens,
		Temperature: s.config.LLM.Temperature,
	}

	var lastErr error
	for attempt := 0; attempt <= consts.LLMMaxRetries; attempt++ {
		if attempt > 0 {
			delay := backoffWithJitter(attempt)
			s.logger.Warn("retrying llm generate",
				zap.Int("attempt", attempt), zap.Duration("delay", delay), zap.Error(lastErr))
			select {
			case <-callCtx.Done():
				return models.GenerateResponse{}, callCtx.Err()
			case <-time.After(delay):
			}
		}

		start := time.Now()
		resp, err := s.provider.Generate(callCtx, req)
		duration := time.Since(start)

		if err == nil {
			cost := s.estimateCost(resp.Usage)
			s.logger.Info(
				"llm generate succeeded",
				zap.String("model", s.config.LLM.Model),
				zap.Int("prompt_tokens", resp.Usage.PromptTokens),
				zap.Int("completion_tokens", resp.Usage.CompletionTokens),
				zap.Duration("duration", duration),
				zap.Float64("estimated_cost_usd", cost),
			)
			return resp, nil
		}

		lastErr = err
		var provErr *ProviderError
		if errors.As(err, &provErr) && !provErr.Retryable() {
			return models.GenerateResponse{}, err
		}
	}

	return models.GenerateResponse{}, fmt.Errorf("llm generate failed after %d attempts: %w", consts.LLMMaxRetries+1, lastErr)
}

func (s *Service) estimateCost(usage models.TokenUsage) float64 {
	inputCost := float64(usage.PromptTokens) / 1_000_000 * s.pricing.Input
	outputCost := float64(usage.CompletionTokens) / 1_000_000 * s.pricing.Output
	return inputCost + outputCost
}

func backoffWithJitter(attempt int) time.Duration {
	backoff := float64(consts.LLMBaseBackoff) * math.Pow(2, float64(attempt-1))
	if backoff > float64(consts.LLMMaxBackoff) {
		backoff = float64(consts.LLMMaxBackoff)
	}
	return time.Duration(backoff * (0.5 + rand.Float64()*0.5))
}
