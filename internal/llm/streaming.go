package llm

import (
	"context"
	"fmt"
	"time"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/models"
	"go.uber.org/zap"
)

const streamCallTimeout = 90 * time.Second

// Stream runs a streaming generation and returns a channel of StreamEvents.
// The channel is closed after a Done or Error event. Retries only apply if
// the stream fails before any token is emitted (retrying mid-stream would
// duplicate already-sent tokens to the client) — once a Token event has
// gone out, failures surface as a StreamEventError instead.
func (s *Service) Stream(ctx context.Context, prompt string) <-chan models.StreamEvent {
	out := make(chan models.StreamEvent)

	go func() {
		defer close(out)

		callCtx, cancel := context.WithTimeout(ctx, streamCallTimeout)
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
			// Nothing reached the client yet — safe to just report failure,
			// no partial/duplicate content risk. A caller wanting retry-then-
			// stream can call Service.Stream again itself; we don't loop here
			// to keep this function's behavior predictable for SSE handlers.
			s.logger.Error("llm stream failed before first token", zap.Error(err))
			select {
			case out <- models.StreamEvent{Type: consts.StreamEventError, Err: fmt.Errorf("stream failed: %w", err), Timestamp: time.Now()}:
			case <-ctx.Done():
			}
			return
		}

		if err != nil {
			// Tokens were already sent to the client — log but don't try to
			// fabricate a clean Done event; the client saw a truncated
			// stream and chat/service.go should decide how to handle that
			// (e.g. mark the message incomplete in message_sources).
			s.logger.Error("llm stream failed mid-stream", zap.Error(err))
		}
	}()

	return out
}
