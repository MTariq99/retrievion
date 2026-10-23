package ingestion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/models"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

type VersionForIngestion struct {
	TenantID   uuid.UUID
	DocumentID uuid.UUID
	StorageKey string
	MimeType   string
}

func (r *Repository) GetVersionForIngestion(ctx context.Context, versionID uuid.UUID) (*VersionForIngestion, error) {
	const query = `
		SELECT d.tenant_id, dv.document_id, dv.storage_key, d.mime_type
		FROM document_versions dv
		INNER JOIN documents d ON d.id = dv.document_id
		WHERE dv.id = $1
	`
	var v VersionForIngestion
	err := r.db.QueryRowContext(ctx, query, versionID).Scan(
		&v.TenantID, &v.DocumentID, &v.StorageKey, &v.MimeType,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("version %s not found: %w", versionID, err)
		}
		return nil, fmt.Errorf("get version for ingestion: %w", err)
	}
	return &v, nil
}

func (r *Repository) MarkJobProcessing(ctx context.Context, jobID uuid.UUID) error {
	const query = `
		UPDATE ingestion_jobs
		SET status = $2, attempts = attempts + 1, started_at = now()
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query, jobID, consts.JobStatusProcessing)
	if err != nil {
		return fmt.Errorf("mark job processing: %w", err)
	}
	return nil
}

func (r *Repository) MarkJobCompleted(ctx context.Context, jobID uuid.UUID) error {
	const query = `
		UPDATE ingestion_jobs
		SET status = $2, completed_at = now(), error = NULL
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query, jobID, consts.JobStatusCompleted)
	if err != nil {
		return fmt.Errorf("mark job completed: %w", err)
	}
	return nil
}

func (r *Repository) MarkJobFailed(ctx context.Context, jobID uuid.UUID, cause error) error {
	const query = `
		UPDATE ingestion_jobs
		SET status = $2, error = $3
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query, jobID, consts.JobStatusFailed, cause.Error())
	if err != nil {
		return fmt.Errorf("mark job failed: %w", err)
	}
	return nil
}

func (r *Repository) StoreChunks(ctx context.Context, tenantID, documentID uuid.UUID, chunks []models.Chunk, embeddings map[uuid.UUID][]float32) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	const query = `
		INSERT INTO document_chunks (
			id,
			tenant_id,
			document_id,
			document_version_id,
			parent_chunk_id,
			chunk_index,
			content,
			content_hash,
			embedding,
			page,
			section,
			token_count
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	seen := make(map[string]uuid.UUID)

	for _, c := range chunks {
		// Parent and child chunks are allowed to have the same content.
		// Only reject duplicate chunks within the same parent/child scope.
		var key string
		if c.ParentChunkID != nil {
			key = fmt.Sprintf("%s:%s", c.ParentChunkID.String(), c.ContentHash)
		} else {
			key = "parent:" + c.ContentHash
		}

		if existing, ok := seen[key]; ok {
			return fmt.Errorf(
				"duplicate chunk content: hash=%s, chunks=%s and %s, content=%q",
				c.ContentHash,
				existing,
				c.ID,
				c.Content,
			)
		}

		seen[key] = c.ID
	}
	for _, c := range chunks {
		var embedding any // nil for parent chunks (no embedding)
		if v, ok := embeddings[c.ID]; ok {
			embedding = pgVector(v)
		}

		var page any
		if c.Page > 0 {
			page = c.Page
		}
		var section any
		if c.Section != "" {
			section = c.Section
		}

		_, err := tx.ExecContext(
			ctx,
			query,
			c.ID,
			tenantID,
			documentID,
			c.DocumentVersionID,
			c.ParentChunkID,
			c.ChunkIndex,
			c.Content,
			c.ContentHash,
			embedding,
			page,
			section,
			c.TokenCount,
		)
		if err != nil {
			return fmt.Errorf("insert chunk %s: %w", c.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit chunks: %w", err)
	}
	return nil
}

func (r *Repository) ActivateVersion(ctx context.Context, documentID, versionID uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE document_versions
		 SET status = $2
		 WHERE document_id = $1 AND status = $3`,
		documentID,
		consts.DocumentStatusArchived,
		consts.DocumentStatusActive,
	); err != nil {
		return fmt.Errorf("archive previous versions: %w", err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE document_versions
		 SET status = $2
		 WHERE id = $1`,
		versionID,
		consts.DocumentStatusActive,
	); err != nil {
		return fmt.Errorf("activate version: %w", err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE documents
		 SET status = $2, updated_at = now()
		 WHERE id = $1`,
		documentID,
		consts.DocumentStatusActive,
	); err != nil {
		return fmt.Errorf("update document status: %w", err)
	}

	return tx.Commit()
}

func pgVector(v []float32) string {
	s := "["
	for i, f := range v {
		if i > 0 {
			s += ","
		}
		s += fmt.Sprintf("%g", f)
	}
	return s + "]"
}
