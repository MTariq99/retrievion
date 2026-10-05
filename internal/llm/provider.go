package llm

import (
	"context"

	"github.com/mtariq99/retrievion/models"
)

// Provider abstracts a single LLM backend (README §35). Implementations
// should do exactly one thing — call the model — and leave retries,
// timeouts-beyond-a-single-call, model selection, and cost tracking to
// Service.
type Provider interface {
	Generate(ctx context.Context, req models.GenerateRequest) (models.GenerateResponse, error)
	Stream(ctx context.Context, req models.GenerateRequest, callback func(models.StreamEvent) error) error
}

// Compile-time check that Client satisfies Provider.
var _ Provider = (*Client)(nil)
