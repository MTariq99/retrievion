package retrieval

import (
	"context"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/models"
)

type VectorSearcher interface {
	Search(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, queryEmbedding []float32, limit int) ([]models.RetrievedChunk, error)
}
