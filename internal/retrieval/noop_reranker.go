// internal/retrieval/noop_reranker.go
package retrieval

import (
	"context"

	"github.com/mtariq99/retrievion/models"
)

type NoopReranker struct{}

func NewNoopReranker() *NoopReranker {
	return &NoopReranker{}
}

func (r *NoopReranker) Rerank(ctx context.Context, query string, candidates []models.RetrievedChunk) ([]models.RetrievedChunk, error) {
	return candidates, nil
}
