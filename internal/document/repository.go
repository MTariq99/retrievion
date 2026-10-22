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

// querier is satisfied by both *sql.DB and *sql.Tx, so repository
// methods work identically whether called inside or outside a transaction.
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

// WithTx runs fn inside a PostgreSQL transaction and passes back a
// documentMutator scoped to that transaction. Commits on success,
// rolls back on error or panic.
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

func (r *Repository) FindByUserID(ctx context.Context, userID, tenantID uuid.UUID) ([]*models.Document, error) {
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
			dv.created_at

		FROM documents d
		INNER JOIN document_versions dv
			ON dv.document_id = d.id

		WHERE d.user_id = $1
		AND d.tenant_id = $2 AND d.deleted_at IS NULL

		ORDER BY d.created_at DESC, dv.version DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, errs.RepositoryError(err)
	}
	documents := make([]*models.Document, 0)
	documentMap := make(map[uuid.UUID]*models.Document)
	defer rows.Close()
	for rows.Next() {
		var (
			documentID uuid.UUID
			tenantID   uuid.UUID
			name       string
			mimeType   string
			status     consts.DocumentStatus
			createdAt  time.Time
			updatedAt  time.Time

			versionID         uuid.UUID
			versionDocumentID uuid.UUID
			versionNumber     int
			versionCreatedAt  time.Time
		)

		if err := rows.Scan(
			&documentID,
			&tenantID,
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
				TenantID:  tenantID,
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

		document.Versions = append(
			document.Versions,
			&models.DocumentVersion{
				ID:         versionID,
				DocumentID: versionDocumentID,
				Version:    versionNumber,
				CreatedAt:  versionCreatedAt,
			},
		)
		if err := rows.Err(); err != nil {
			return nil, errs.RepositoryError(err)
		}
	}
	return documents, nil
}

func (r *Repository) FindDocument(ctx context.Context, documentID uuid.UUID) (*models.Document, error) {
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

		WHERE d.id = $1 AND deleted_at is NULL
		LIMIT 1
	`

	var document models.Document
	var version models.DocumentVersion

	err := r.q.QueryRowContext(
		ctx,
		query,
		documentID,
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

func (r *Repository) DeleteDocument(ctx context.Context, userID, tenantID, documentID uuid.UUID) error {
	if userID == (uuid.UUID{}) ||
		tenantID == (uuid.UUID{}) ||
		documentID == (uuid.UUID{}) {
		return errs.ErrMissingRequiredFields
	}

	const query = `
		UPDATE documents
		SET deleted_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
		  AND user_id = $2
		  AND tenant_id = $3
		  AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		documentID,
		userID,
		tenantID,
	)
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
