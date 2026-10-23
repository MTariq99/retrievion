package llm

import (
	"context"
	"fmt"
	"time"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/models"
	"go.uber.org/zap"
)

func (s *Service) Stream(ctx context.Context, prompt string) <-chan models.StreamEvent {
	out := make(chan models.StreamEvent)

	go func() {
		defer close(out)

		callCtx, cancel := context.WithTimeout(ctx, consts.LLMStreamCallTimeout)
		defer cancel()

		req := models.GenerateRequest{
			Prompt:      prompt,
			MaxTokens:   s.config.LLM.MaxTokens,
			Temperature: s.config.LLM.Temperature,
		}

		tokensSent := false

		err := s.provider.Stream(callCtx, req, func(ev models.StreamEvent) error {
			if ev.Type == consts.StreamEventToken {
				tokensSent = true
			}
			select {
			case out <- ev:
				return nil
			case <-callCtx.Done():
				return callCtx.Err()
			}
		})

		if err != nil && !tokensSent {

			s.logger.Error("llm stream failed before first token", zap.Error(err))
			select {
			case out <- models.StreamEvent{Type: consts.StreamEventError, Err: fmt.Errorf("stream failed: %w", err), Timestamp: time.Now()}:
			case <-ctx.Done():
			}
			return
		}

		if err != nil {
			s.logger.Error("llm stream failed mid-stream", zap.Error(err))
		}
	}()

	return out
}
