package document

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
)

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Repository struct {
	db *sql.DB
	q  querier
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db, q: db}
}

func (r *Repository) WithTx(ctx context.Context, fn func(tx documentMutator) error) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				err = fmt.Errorf("%w (rollback also failed: %v)", err, rbErr)
			}
			return
		}
		err = tx.Commit()
	}()

	txRepo := &Repository{db: r.db, q: tx}
	err = fn(txRepo)
	return err
}

func (r *Repository) FindBySHA256(ctx context.Context, tenantID uuid.UUID, hash string) (*models.Document, error) {
	const query = `
		SELECT
			d.id, d.tenant_id, d.name, d.mime_type, d.status, d.created_at, d.updated_at
		FROM documents d
		INNER JOIN document_versions dv ON dv.document_id = d.id
		WHERE d.tenant_id = $1 AND dv.content_hash = $2
		LIMIT 1
	`

	var document models.Document

	err := r.q.QueryRowContext(ctx, query, tenantID, hash).Scan(
		&document.ID,
		&document.TenantID,
		&document.Name,
		&document.MimeType,
		&document.Status,
		&document.CreatedAt,
		&document.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find document by sha256: %w", err)
	}

	return &document, nil
}

func (r *Repository) CreateDocument(ctx context.Context, document *models.Document) error {
	const query = `
		INSERT INTO documents (id, tenant_id, name, mime_type, status)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := r.q.ExecContext(
		ctx, query,
		document.ID,
		document.TenantID,
		document.Name,
		document.MimeType,
		document.Status,
	)
	if err != nil {
		return fmt.Errorf("create document: %w", err)
	}

	return nil
}

func (r *Repository) CreateDocumentVersion(ctx context.Context, version *models.DocumentVersion) error {
	const query = `
		INSERT INTO document_versions (id, document_id, version, content_hash, storage_key, status)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := r.q.ExecContext(
		ctx, query,
		version.ID,
		version.DocumentID,
		version.Version,
		version.ContentHash,
		version.StorageKey,
		version.Status,
	)
	if err != nil {
		return fmt.Errorf("create document version: %w", err)
	}

	return nil
}

func (r *Repository) CreateIngestionJob(ctx context.Context, job *models.IngestionJob) error {
	const query = `
		INSERT INTO ingestion_jobs (id, tenant_id, document_id, document_version_id)
		VALUES ($1, $2, $3, $4)
	`

	_, err := r.q.ExecContext(
		ctx, query,
		job.ID,
		job.TenantID,
		job.DocumentID,
		job.DocumentVersionID,
	)
	if err != nil {
		return fmt.Errorf("create ingestion job: %w", err)
	}

	return nil
}

func (r *Repository) FindByUserID(ctx context.Context, _ uuid.UUID, tenantID uuid.UUID) ([]*models.Document, error) {
	if tenantID == uuid.Nil {
		return nil, errs.ErrMissingRequiredFields
	}

	const query = `
		SELECT
			d.id, d.tenant_id, d.name, d.mime_type, d.status,
			d.created_at, d.updated_at,
			dv.id, dv.document_id, dv.version, dv.created_at
		FROM documents d
		LEFT JOIN document_versions dv ON dv.document_id = d.id
		WHERE d.tenant_id = $1 AND d.deleted_at IS NULL
		ORDER BY d.created_at DESC, dv.version DESC
	`

	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, errs.RepositoryError(err)
	}
	defer rows.Close()

	documents := make([]*models.Document, 0)
	documentMap := make(map[uuid.UUID]*models.Document)

	for rows.Next() {
		var (
			documentID        uuid.UUID
			rowTenantID       uuid.UUID
			name              string
			mimeType          string
			status            consts.DocumentStatus
			createdAt         time.Time
			updatedAt         time.Time
			versionID         sql.NullString
			versionDocumentID sql.NullString
			versionNumber     sql.NullInt64
			versionCreatedAt  sql.NullTime
		)

		if err := rows.Scan(
			&documentID,
			&rowTenantID,
			&name,
			&mimeType,
			&status,
			&createdAt,
			&updatedAt,
			&versionID,
			&versionDocumentID,
			&versionNumber,
			&versionCreatedAt,
		); err != nil {
			return nil, errs.RepositoryError(err)
		}

		document, exists := documentMap[documentID]
		if !exists {
			document = &models.Document{
				ID:        documentID,
				TenantID:  rowTenantID,
				Name:      name,
				MimeType:  mimeType,
				Status:    status,
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
				Versions:  make([]*models.DocumentVersion, 0),
			}
			documentMap[documentID] = document
			documents = append(documents, document)
		}

		if versionID.Valid {
			versionUUID, err := uuid.Parse(versionID.String)
			if err != nil {
				return nil, errs.RepositoryError(err)
			}
			versionDocUUID, err := uuid.Parse(versionDocumentID.String)
			if err != nil {
				return nil, errs.RepositoryError(err)
			}

			document.Versions = append(document.Versions, &models.DocumentVersion{
				ID:         versionUUID,
				DocumentID: versionDocUUID,
				Version:    int(versionNumber.Int64),
				CreatedAt:  versionCreatedAt.Time,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, errs.RepositoryError(err)
	}

	return documents, nil
}

func (r *Repository) FindDocument(ctx context.Context, tenantID, documentID uuid.UUID) (*models.Document, error) {
	const query = `
		SELECT
			d.id,
			d.tenant_id,
			d.name,
			d.mime_type,
			d.status,
			d.created_at,
			d.updated_at,

			dv.id,
			dv.document_id,
			dv.version,
			dv.content_hash,
			dv.storage_key,
			dv.status,
			dv.created_at,
			dv.updated_at

		FROM documents d
		INNER JOIN document_versions dv
			ON dv.document_id = d.id

				WHERE d.id = $1
				AND d.tenant_id = $2
				AND d.deleted_at IS NULL
				LIMIT 1
	`

	var document models.Document
	var version models.DocumentVersion

	err := r.q.QueryRowContext(
		ctx,
		query,
		documentID,
		tenantID,
	).Scan(
		&document.ID,
		&document.TenantID,
		&document.Name,
		&document.MimeType,
		&document.Status,
		&document.CreatedAt,
		&document.UpdatedAt,

		&version.ID,
		&version.DocumentID,
		&version.Version,
		&version.ContentHash,
		&version.StorageKey,
		&version.Status,
		&version.CreatedAt,
		&version.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("find document: %w", err)
	}

	document.Versions = []*models.DocumentVersion{
		&version,
	}

	return &document, nil
}

func (r *Repository) DeleteDocument(ctx context.Context, tenantID, documentID uuid.UUID) error {
	if tenantID == uuid.Nil || documentID == uuid.Nil {
		return errs.ErrMissingRequiredFields
	}

	const query = `
		UPDATE documents
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1
		  AND tenant_id = $2
		  AND deleted_at IS NULL
	`

	result, err := r.q.ExecContext(ctx, query, documentID, tenantID)
	if err != nil {
		return errs.RepositoryError(err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errs.RepositoryError(err)
	}
	if rowsAffected == 0 {
		return errs.ErrNotFound
	}

	return nil
}

func (r *Repository) CreateOutboxEvent(ctx context.Context, eventID uuid.UUID, eventType string, aggregateID uuid.UUID, payload []byte) error {
	const query = `
        INSERT INTO ingestion_outbox (
            id, event_type, aggregate_id, payload
        )
        VALUES ($1, $2, $3, $4::jsonb)
    `

	if _, err := r.q.ExecContext(
		ctx,
		query,
		eventID,
		eventType,
		aggregateID,
		string(payload),
	); err != nil {
		return fmt.Errorf("create outbox event: %w", err)
	}

	return nil
}

type OutboxEvent struct {
	ID          uuid.UUID
	EventType   string
	AggregateID uuid.UUID
	Payload     []byte
	Attempts    int
	LockedUntil time.Time
}

func (r *Repository) ClaimOutboxEvents(ctx context.Context, limit int, lease time.Duration) ([]OutboxEvent, error) {
	const query = `
		WITH candidates AS (
			SELECT id
			FROM ingestion_outbox
			WHERE published_at IS NULL
			  AND dead_letter_at IS NULL
			  AND next_attempt_at <= now()
			  AND (locked_until IS NULL OR locked_until <= now())
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE ingestion_outbox AS o
		SET locked_until = now() + $2::interval,
		    attempts = o.attempts + 1
		FROM candidates AS c
		WHERE o.id = c.id
		RETURNING o.id, o.event_type, o.aggregate_id,
		          o.payload, o.attempts, o.locked_until
	`

	rows, err := r.db.QueryContext(ctx, query, limit, lease.String())
	if err != nil {
		return nil, fmt.Errorf("claim outbox events: %w", err)
	}
	defer rows.Close()

	var events []OutboxEvent
	for rows.Next() {
		var event OutboxEvent
		if err := rows.Scan(
			&event.ID,
			&event.EventType,
			&event.AggregateID,
			&event.Payload,
			&event.Attempts,
			&event.LockedUntil,
		); err != nil {
			return nil, fmt.Errorf("scan outbox event: %w", err)
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox events: %w", err)
	}

	return events, nil
}

func (r *Repository) MarkOutboxEventPublished(ctx context.Context, event OutboxEvent) error {
	const query = `
		UPDATE ingestion_outbox
		SET published_at = now(),
		    locked_until = NULL,
		    last_error = NULL
		WHERE id = $1
		  AND locked_until = $2
		  AND published_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, event.ID, event.LockedUntil)
	if err != nil {
		return fmt.Errorf("mark outbox event published: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check published outbox event: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("outbox event %s lease expired or was replaced", event.ID)
	}

	return nil
}

func (r *Repository) MarkOutboxEventFailed(ctx context.Context, event OutboxEvent, publishErr error, retryDelay time.Duration, maxAttempts int) error {
	const query = `
		UPDATE ingestion_outbox
		SET last_error = $3,
		    next_attempt_at = CASE
		        WHEN attempts >= $5 THEN next_attempt_at
		        ELSE now() + $4::interval
		    END,
		    dead_letter_at = CASE
		        WHEN attempts >= $5 THEN now()
		        ELSE NULL
		    END,
		    locked_until = NULL
		WHERE id = $1
		  AND locked_until = $2
		  AND published_at IS NULL
		  AND dead_letter_at IS NULL
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		event.ID,
		event.LockedUntil,
		publishErr.Error(),
		retryDelay.String(),
		maxAttempts,
	)
	if err != nil {
		return fmt.Errorf("record outbox publish failure: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check failed outbox event: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("outbox event %s lease expired or was replaced", event.ID)
	}

	return nil
}
