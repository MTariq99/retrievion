package retrieval

import (
	"context"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/models"
)

type KeywordSearcher interface {
	Search(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, query string, limit int) ([]models.RetrievedChunk, error)
}
