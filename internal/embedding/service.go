package embedding

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
	"go.uber.org/zap"
)

type Service struct {
	provider Provider
	logger   *zap.Logger
}

func NewService(provider Provider, logger *zap.Logger) *Service {
	return &Service{provider: provider, logger: logger}
}

func (s *Service) Metadata() models.EmbeddingModelMetadata {
	return s.provider.Metadata()
}

func (s *Service) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, errs.ErrEmptyInput
	}

	vectors := make([][]float32, 0, len(texts))

	for start := 0; start < len(texts); start += consts.ServiceBatchCap {
		end := start + consts.ServiceBatchCap
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[start:end]

		batchVectors, err := s.embedWithRetry(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("embed batch [%d:%d]: %w", start, end, err)
		}

		vectors = append(vectors, batchVectors...)
	}

	if len(vectors) != len(texts) {
		return nil, errs.ErrDimensionMismatch
	}

	return vectors, nil
}

func (s *Service) embedWithRetry(ctx context.Context, batch []string) ([][]float32, error) {
	var lastErr error

	for attempt := 0; attempt <= consts.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := backoffWithJitter(attempt)
			s.logger.Warn("retrying embedding request", zap.Int("attempt", attempt), zap.Duration("delay", delay), zap.Error(lastErr))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		start := time.Now()
		vectors, err := s.provider.Embed(ctx, batch)
		duration := time.Since(start)

		if err == nil {
			s.logger.Debug("embedding batch succeeded", zap.Int("batch_size", len(batch)), zap.Duration("duration", duration))
			return vectors, nil
		}

		lastErr = err

		var provErr *ProviderError
		if errors.As(err, &provErr) && !provErr.Retryable() {
			return nil, err
		}
	}

	return nil, fmt.Errorf("embedding failed after %d attempts: %w", consts.MaxRetries+1, lastErr)
}

func backoffWithJitter(attempt int) time.Duration {
	backoff := float64(consts.BaseBackoff) * math.Pow(2, float64(attempt-1))
	if backoff > float64(consts.MaxBackoff) {
		backoff = float64(consts.MaxBackoff)
	}
	jitter := backoff * (0.5 + rand.Float64()*0.5) // 50–100% of computed backoff
	return time.Duration(jitter)
}
