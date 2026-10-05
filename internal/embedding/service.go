package embedding

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/mtariq99/retrievion/models"
	"go.uber.org/zap"
)

const (
	maxRetries      = 4
	baseBackoff     = 500 * time.Millisecond
	maxBackoff      = 8 * time.Second
	serviceBatchCap = geminiMaxBatchSize // mirror provider's per-call limit
)

// Service wraps a Provider with application-level concerns: chunking
// arbitrarily large input into provider-sized batches, retrying transient
// errors with exponential backoff + jitter, and logging metrics. This is
// what ingestion/pipeline.go and retrieval/service.go should depend on —
// not Provider directly — so callers don't need to know about batch limits
// or retry policy.
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

// Embed batches texts into provider-sized chunks, embeds each with retry,
// and returns vectors in the same order as the input.
func (s *Service) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, ErrEmptyInput
	}

	vectors := make([][]float32, 0, len(texts))

	for start := 0; start < len(texts); start += serviceBatchCap {
		end := start + serviceBatchCap
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
		return nil, ErrDimensionMismatch
	}

	return vectors, nil
}

func (s *Service) embedWithRetry(ctx context.Context, batch []string) ([][]float32, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
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
			return nil, err // non-retryable: bad request, auth failure, etc.
		}
		// unknown error types (network errors, timeouts) are treated as
		// retryable by default, consistent with README §50's transient list
	}

	return nil, fmt.Errorf("embedding failed after %d attempts: %w", maxRetries+1, lastErr)
}

func backoffWithJitter(attempt int) time.Duration {
	backoff := float64(baseBackoff) * math.Pow(2, float64(attempt-1))
	if backoff > float64(maxBackoff) {
		backoff = float64(maxBackoff)
	}
	jitter := backoff * (0.5 + rand.Float64()*0.5) // 50–100% of computed backoff
	return time.Duration(jitter)
}
