package retrieval

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/models"
)

type VectorRepository struct {
	db *sql.DB
}
type KeywordRepository struct {
	db *sql.DB
}

func NewVectorRepository(db *sql.DB) *VectorRepository {
	return &VectorRepository{
		db: db,
	}
}

func NewKeywordRepository(db *sql.DB) *KeywordRepository {
	return &KeywordRepository{
		db: db,
	}
}

func (r *VectorRepository) Search(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, queryEmbedding []float32, limit int) ([]models.RetrievedChunk, error) {
	const query = `
		SELECT
			dc.id,
			dc.document_id,
			dc.document_version_id,
			d.name,
			dc.content,
			dc.page,
			dc.section,
			1 - (dc.embedding <=> $1::vector) AS score
		FROM document_chunks dc
		INNER JOIN documents d
			ON d.id = dc.document_id
		INNER JOIN document_versions dv
			ON dv.id = dc.document_version_id
		WHERE
			dc.tenant_id = $2
			AND dc.embedding IS NOT NULL
			AND d.deleted_at IS NULL
			AND dv.status = $4
		ORDER BY dc.embedding <=> $1::vector
		LIMIT $3
	`

	embedding := vectorToString(queryEmbedding)

	rows, err := r.db.QueryContext(
		ctx,
		query,
		embedding,
		tenantID,
		limit,
		consts.DocumentStatusActive,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]models.RetrievedChunk, 0, limit)

	for rows.Next() {
		var chunk models.RetrievedChunk

		if err := rows.Scan(
			&chunk.ChunkID,
			&chunk.DocumentID,
			&chunk.DocumentVersionID,
			&chunk.DocumentName,
			&chunk.Content,
			&chunk.Page,
			&chunk.Section,
			&chunk.Score,
		); err != nil {
			return nil, err
		}

		chunk.VectorScore = chunk.Score
		results = append(results, chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func (r *KeywordRepository) Search(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID, queryText string, limit int) ([]models.RetrievedChunk, error) {
	const query = `
		SELECT
			dc.id,
			dc.document_id,
			dc.document_version_id,
			dc.content,
			dc.page,
			dc.section,
			ts_rank_cd(
				to_tsvector('english', dc.content),
				plainto_tsquery('english', $1)
			) AS score
		FROM document_chunks dc
		INNER JOIN documents d
			ON d.id = dc.document_id
		INNER JOIN document_versions dv
			ON dv.id = dc.document_version_id
		WHERE
			dc.tenant_id = $2
			AND d.deleted_at IS NULL
			AND dv.status = $4
			AND to_tsvector(
				'english',
				dc.content
			) @@ plainto_tsquery(
				'english',
				$1
			)
		ORDER BY score DESC
		LIMIT $3
	`

	rows, err := r.db.QueryContext(
		ctx,
		query,
		queryText,
		tenantID,
		limit,
		consts.DocumentStatusActive,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]models.RetrievedChunk, 0, limit)

	for rows.Next() {
		var chunk models.RetrievedChunk

		if err := rows.Scan(
			&chunk.ChunkID,
			&chunk.DocumentID,
			&chunk.DocumentVersionID,
			&chunk.Content,
			&chunk.Page,
			&chunk.Section,
			&chunk.Score,
		); err != nil {
			return nil, err
		}

		chunk.KeywordScore = chunk.Score
		results = append(results, chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func vectorToString(vector []float32) string {
	values := make([]string, len(vector))

	for i, value := range vector {
		values[i] = fmt.Sprintf("%f", value)
	}

	return "[" + strings.Join(values, ",") + "]"
}
