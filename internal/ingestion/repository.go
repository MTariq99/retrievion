package ingestion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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

func (r *Repository) MarkJobProcessing(ctx context.Context, jobID uuid.UUID) (int, error) {
	const query = `
		UPDATE ingestion_jobs
		SET status = $2,
		    attempts = attempts + 1,
		    started_at = now(),
		    completed_at = NULL,
		    error = NULL
		WHERE id = $1 AND status = $3
		RETURNING attempts
	`

	var attempt int
	err := r.db.QueryRowContext(
		ctx,
		query,
		jobID,
		consts.JobStatusProcessing,
		consts.JobStatusQueued,
	).Scan(&attempt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("claim ingestion job %s: job does not exist or is not queued", jobID)
		}
		return 0, fmt.Errorf("claim ingestion job %s: %w", jobID, err)
	}

	return attempt, nil
}

func (r *Repository) MarkJobCompleted(ctx context.Context, jobID uuid.UUID, attempt int) error {
	const query = `
		UPDATE ingestion_jobs
		SET status = $2, completed_at = now(), error = NULL
		WHERE id = $1 AND status = $3 AND attempts = $4
	`
	result, err := r.db.ExecContext(
		ctx, query,
		jobID,
		consts.JobStatusCompleted,
		consts.JobStatusProcessing,
		attempt,
	)
	if err != nil {
		return fmt.Errorf("mark job completed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check job completion result: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("mark job completed: job %s does not exist or is not processing", jobID)
	}
	return nil
}

func (r *Repository) MarkJobFailed(ctx context.Context, jobID uuid.UUID, attempt int, cause error) error {
	if cause == nil {
		return fmt.Errorf("mark job failed: cause is required")
	}

	const query = `
	UPDATE ingestion_jobs
	SET status = $2, error = $3
	WHERE id = $1 AND status = $4 AND attempts = $5
	`
	result, err := r.db.ExecContext(
		ctx,
		query,
		jobID,
		consts.JobStatusFailed,
		cause.Error(),
		consts.JobStatusProcessing,
		attempt,
	)
	if err != nil {
		return fmt.Errorf("mark job failed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check job failure result: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("mark job failed: job %s does not exist or is not processing", jobID)
	}
	return nil
}

func (r *Repository) StoreChunks(
	ctx context.Context,
	tenantID, documentID uuid.UUID,
	chunks []models.Chunk,
	embeddings map[uuid.UUID][]float32,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin chunk transaction: %w", err)
	}
	defer tx.Rollback()

	// Detect duplicate content within the incoming batch before writing.
	seen := make(map[string]uuid.UUID, len(chunks))
	for _, c := range chunks {
		if c.ID == uuid.Nil {
			return fmt.Errorf("store chunks: chunk ID is required")
		}

		var key string
		if c.ParentChunkID != nil {
			key = fmt.Sprintf("%s:%s", c.ParentChunkID.String(), c.ContentHash)
		} else {
			key = "parent:" + c.ContentHash
		}

		if existing, ok := seen[key]; ok {
			return fmt.Errorf(
				"duplicate chunk content: hash=%s, chunks=%s and %s",
				c.ContentHash, existing, c.ID,
			)
		}
		seen[key] = c.ID
	}

	const insertQuery = `
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
		ON CONFLICT DO NOTHING
	`

	// Match the expression used by document_chunks_unique_content_idx.
	const lookupQuery = `
		SELECT id, tenant_id, document_id, content,
		       embedding IS NULL
		FROM document_chunks
		WHERE document_version_id = $1
		  AND COALESCE(
		        parent_chunk_id,
		        '00000000-0000-0000-0000-000000000000'::uuid
		      ) = COALESCE(
		        $2::uuid,
		        '00000000-0000-0000-0000-000000000000'::uuid
		      )
		  AND content_hash = $3
	`

	const repairEmbeddingQuery = `
		UPDATE document_chunks
		SET embedding = $2
		WHERE id = $1 AND embedding IS NULL
	`

	// Maps the chunker's temporary IDs to the IDs actually persisted.
	// Children must reference the persisted parent ID, not a discarded ID.
	resolvedIDs := make(map[uuid.UUID]uuid.UUID, len(chunks))

	for _, c := range chunks {
		var parentID *uuid.UUID
		var parentArg any

		if c.ParentChunkID != nil {
			resolvedParentID, ok := resolvedIDs[*c.ParentChunkID]
			if !ok {
				return fmt.Errorf(
					"store chunk %s: parent chunk %s was not resolved first",
					c.ID, *c.ParentChunkID,
				)
			}

			parentID = &resolvedParentID
			parentArg = resolvedParentID
		}

		var embedding any
		if c.ParentChunkID != nil {
			vector, ok := embeddings[c.ID]
			if !ok || len(vector) == 0 {
				return fmt.Errorf(
					"store child chunk %s: embedding is missing",
					c.ID,
				)
			}
			embedding = pgVector(vector)
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
			insertQuery,
			c.ID,
			tenantID,
			documentID,
			c.DocumentVersionID,
			parentID,
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

		// Whether this call inserted the row or lost a conflict, resolve
		// the canonical row through the database's content-based identity.
		var (
			storedID         uuid.UUID
			storedTenantID   uuid.UUID
			storedDocumentID uuid.UUID
			storedContent    string
			embeddingIsNull  bool
		)

		err = tx.QueryRowContext(
			ctx,
			lookupQuery,
			c.DocumentVersionID,
			parentArg,
			c.ContentHash,
		).Scan(
			&storedID,
			&storedTenantID,
			&storedDocumentID,
			&storedContent,
			&embeddingIsNull,
		)
		if err != nil {
			return fmt.Errorf(
				"resolve persisted chunk %s: %w",
				c.ID, err,
			)
		}

		if storedTenantID != tenantID || storedDocumentID != documentID {
			return fmt.Errorf(
				"resolve chunk %s: existing chunk belongs to a different tenant or document",
				c.ID,
			)
		}

		if storedContent != c.Content {
			return fmt.Errorf(
				"resolve chunk %s: content hash collision or inconsistent stored content",
				c.ID,
			)
		}

		resolvedIDs[c.ID] = storedID

		// Repair legacy/incomplete child rows without replacing a vector
		// that was already stored successfully.
		if c.ParentChunkID != nil && embeddingIsNull {
			_, err = tx.ExecContext(
				ctx,
				repairEmbeddingQuery,
				storedID,
				embedding,
			)
			if err != nil {
				return fmt.Errorf(
					"repair embedding for chunk %s: %w",
					storedID, err,
				)
			}

			err = tx.QueryRowContext(
				ctx,
				`SELECT embedding IS NULL FROM document_chunks WHERE id = $1`,
				storedID,
			).Scan(&embeddingIsNull)
			if err != nil {
				return fmt.Errorf(
					"verify embedding for chunk %s: %w",
					storedID, err,
				)
			}
			if embeddingIsNull {
				return fmt.Errorf(
					"repair embedding for chunk %s: embedding remains NULL",
					storedID,
				)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit chunks: %w", err)
	}

	return nil
}

func (r *Repository) ActivateVersion(ctx context.Context, jobID, documentID, versionID uuid.UUID, attempt int) error {
	if jobID == uuid.Nil || documentID == uuid.Nil || versionID == uuid.Nil {
		return fmt.Errorf("activate version: job, document, and version IDs are required")
	}
	if attempt < 1 {
		return fmt.Errorf("activate version: attempt must be positive")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin activation transaction: %w", err)
	}
	defer tx.Rollback()

	var claimedJobID uuid.UUID
	err = tx.QueryRowContext(
		ctx,
		`SELECT id
		 FROM ingestion_jobs
		 WHERE id = $1
		   AND document_id = $2
		   AND document_version_id = $3
		   AND status = $4
		   AND attempts = $5
		 FOR UPDATE`,
		jobID,
		documentID,
		versionID,
		consts.JobStatusProcessing,
		attempt,
	).Scan(&claimedJobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf(
				"activate version: job %s attempt %d is no longer current",
				jobID,
				attempt,
			)
		}
		return fmt.Errorf("lock ingestion attempt before activation: %w", err)
	}

	var targetExists bool
	err = tx.QueryRowContext(
		ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM document_versions
			WHERE id = $1 AND document_id = $2
		)`,
		versionID,
		documentID,
	).Scan(&targetExists)
	if err != nil {
		return fmt.Errorf("validate target version: %w", err)
	}
	if !targetExists {
		return fmt.Errorf(
			"activate version: version %s does not belong to document %s",
			versionID,
			documentID,
		)
	}

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

	result, err := tx.ExecContext(
		ctx,
		`UPDATE document_versions
		 SET status = $3
		 WHERE id = $1 AND document_id = $2`,
		versionID,
		documentID,
		consts.DocumentStatusActive,
	)
	if err != nil {
		return fmt.Errorf("activate target version: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check target activation result: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf(
			"activate version: expected to update one target version, updated %d",
			rowsAffected,
		)
	}

	result, err = tx.ExecContext(
		ctx,
		`UPDATE documents
		 SET status = $2, updated_at = now()
		 WHERE id = $1`,
		documentID,
		consts.DocumentStatusActive,
	)
	if err != nil {
		return fmt.Errorf("update document status: %w", err)
	}

	rowsAffected, err = result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check document status update: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf(
			"activate version: expected to update one document, updated %d",
			rowsAffected,
		)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit version activation: %w", err)
	}

	return nil
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

func (r *Repository) RecoverStaleJobs(ctx context.Context, staleAfter time.Duration, limit int) (int, error) {
	if staleAfter <= 0 {
		return 0, fmt.Errorf("recover stale jobs: timeout must be positive")
	}
	if limit <= 0 {
		return 0, fmt.Errorf("recover stale jobs: limit must be positive")
	}

	const query = `
		WITH stale_jobs AS MATERIALIZED (
			SELECT id, document_id, document_version_id
			FROM ingestion_jobs
			WHERE status = $1
			  AND started_at IS NOT NULL
			  AND started_at < now() - ($2 * INTERVAL '1 second')
			ORDER BY started_at
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		),
		requeued AS (
			UPDATE ingestion_jobs AS j
			SET status = $4,
			    started_at = NULL,
			    error = 'Recovered after processing timeout'
			FROM stale_jobs AS s
			WHERE j.id = s.id
			  AND j.status = $1
			RETURNING j.id, j.document_id, j.document_version_id
		),
		inserted_events AS (
			INSERT INTO ingestion_outbox (
				id,
				event_type,
				aggregate_id,
				payload
			)
			SELECT
				gen_random_uuid(),
				$5,
				r.id,
				jsonb_build_object(
					'job_id', r.id,
					'document_id', r.document_id,
					'version_id', r.document_version_id
				)
			FROM requeued AS r
			RETURNING id
		)
		SELECT count(*) FROM inserted_events
	`

	var recovered int
	err := r.db.QueryRowContext(
		ctx,
		query,
		consts.JobStatusProcessing,
		staleAfter.Seconds(),
		limit,
		consts.JobStatusQueued,
		"ingestion.job.created",
	).Scan(&recovered)
	if err != nil {
		return 0, fmt.Errorf("recover stale ingestion jobs: %w", err)
	}

	return recovered, nil
}
