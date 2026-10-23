package conversation

import (
	"context"
	"database/sql"

	"github.com/google/uuid"

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

func (r *Repository) CreateConversation(ctx context.Context, conversation *models.Conversation) error {
	const query = `
		INSERT INTO conversations (
			id,
			tenant_id,
			user_id,
			title,
			summary,
			created_at,
			updated_at
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			NOW(),
			NOW()
		)
	`

	_, err := r.q.ExecContext(
		ctx,
		query,
		conversation.ID,
		conversation.TenantID,
		conversation.UserID,
		conversation.Title,
		conversation.Summary,
	)
	if err != nil {
		return errs.RepositoryError(err)
	}

	return nil
}

func (r *Repository) GetConversation(ctx context.Context, userID, tenantID, conversationID uuid.UUID) (*models.Conversation, error) {
	const query = `
		SELECT
			id,
			tenant_id,
			user_id,
			title,
			summary,
			created_at,
			updated_at
		FROM conversations
		WHERE id = $1
		  AND user_id = $2
		  AND tenant_id = $3
	`

	conversation := &models.Conversation{}

	err := r.q.QueryRowContext(
		ctx,
		query,
		conversationID,
		userID,
		tenantID,
	).Scan(
		&conversation.ID,
		&conversation.TenantID,
		&conversation.UserID,
		&conversation.Title,
		&conversation.Summary,
		&conversation.CreatedAt,
		&conversation.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errs.ErrNotFound
		}

		return nil, errs.RepositoryError(err)
	}

	return conversation, nil
}

func (r *Repository) ListConversations(ctx context.Context, userID, tenantID uuid.UUID) ([]*models.Conversation, error) {
	const query = `
		SELECT
			id,
			tenant_id,
			user_id,
			title,
			summary,
			created_at,
			updated_at
		FROM conversations
		WHERE user_id = $1
		  AND tenant_id = $2
		ORDER BY updated_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID, tenantID)
	if err != nil {
		return nil, errs.RepositoryError(err)
	}
	defer rows.Close()

	conversations := make([]*models.Conversation, 0)

	for rows.Next() {
		conversation := &models.Conversation{}

		if err := rows.Scan(
			&conversation.ID,
			&conversation.TenantID,
			&conversation.UserID,
			&conversation.Title,
			&conversation.Summary,
			&conversation.CreatedAt,
			&conversation.UpdatedAt,
		); err != nil {
			return nil, errs.RepositoryError(err)
		}

		conversations = append(conversations, conversation)
	}

	if err := rows.Err(); err != nil {
		return nil, errs.RepositoryError(err)
	}

	return conversations, nil
}

func (r *Repository) DeleteConversation(ctx context.Context, userID, tenantID, conversationID uuid.UUID) error {
	const query = `
		DELETE FROM conversations
		WHERE id = $1
		  AND user_id = $2
		  AND tenant_id = $3
	`

	result, err := r.q.ExecContext(
		ctx,
		query,
		conversationID,
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
