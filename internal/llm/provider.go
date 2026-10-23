package llm

import (
	"context"

	"github.com/mtariq99/retrievion/models"
)

type Provider interface {
	Generate(ctx context.Context, req models.GenerateRequest) (models.GenerateResponse, error)
	Stream(ctx context.Context, req models.GenerateRequest, callback func(models.StreamEvent) error) error
}

var _ Provider = (*Client)(nil)
