package retrieval

import (
	"context"

	"github.com/mtariq99/retrievion/models"
)

type Reranker interface {
	Rerank(ctx context.Context, query string, candidates []models.RetrievedChunk) ([]models.RetrievedChunk, error)
}
