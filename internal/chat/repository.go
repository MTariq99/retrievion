package chat

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"github.com/mtariq99/retrievion/internal/consts"
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

func (r *Repository) CreateMessage(ctx context.Context, userID, tenantID, conversationID uuid.UUID, role consts.MessageRole, content string) (*models.Message, error) {
	if userID == uuid.Nil || tenantID == uuid.Nil || conversationID == uuid.Nil {
		return nil, fmt.Errorf("user, tenant, and conversation IDs are required")
	}
	if role == "" {
		return nil, fmt.Errorf("message role is required")
	}
	if content == "" {
		return nil, fmt.Errorf("message content is required")
	}

	const query = `
		INSERT INTO messages (
			id, conversation_id, role, content, created_at
		)
		SELECT $1, c.id, $4, $5, NOW()
		FROM conversations c
		WHERE c.id = $2
		  AND c.user_id = $3
		  AND c.tenant_id = $6
		RETURNING
			id, conversation_id, role, content, token_count, created_at
	`

	messageID := uuid.New()
	var message models.Message

	err := r.db.QueryRowContext(
		ctx,
		query,
		messageID,
		conversationID,
		userID,
		role,
		content,
		tenantID,
	).Scan(
		&message.ID,
		&message.ConversationID,
		&message.Role,
		&message.Content,
		&message.TokenCount,
		&message.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create authorized message: %w", err)
	}

	return &message, nil
}

func (r *Repository) CreateMessageSources(ctx context.Context, userID, tenantID, messageID uuid.UUID, sources []models.MessageSource) error {
	if userID == uuid.Nil || tenantID == uuid.Nil || messageID == uuid.Nil {
		return fmt.Errorf("missing required fields")
	}

	if len(sources) == 0 {
		return nil
	}

	const query = `
		INSERT INTO message_sources (
			id, message_id, chunk_id, document_id, citation_index, created_at
		)
		SELECT $1, m.id, dc.id, d.id, $5, NOW()
		FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		JOIN document_chunks dc ON dc.id = $3
		JOIN documents d ON d.id = $4
		WHERE m.id = $2
		  AND c.user_id = $6
		  AND c.tenant_id = $7
		  AND dc.tenant_id = c.tenant_id
		  AND d.tenant_id = c.tenant_id
		  AND dc.document_id = d.id
	`

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for _, source := range sources {
		result, err := tx.ExecContext(
			ctx,
			query,
			uuid.New(),
			messageID,
			source.ChunkID,
			source.DocumentID,
			source.CitationIndex,
			userID,
			tenantID,
		)
		if err != nil {
			return fmt.Errorf("create message source: %w", err)
		}

		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("check message source insertion: %w", err)
		}
		if affected != 1 {
			return fmt.Errorf(
				"message source rejected: message, chunk, document, or tenant mismatch",
			)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit message sources: %w", err)
	}

	return nil
}

func (r *Repository) GetMessages(ctx context.Context, conversationID uuid.UUID, limit int) ([]*models.Message, error) {
	if conversationID == uuid.Nil {
		return nil, fmt.Errorf("conversation ID is required")
	}

	if limit <= 0 {
		limit = 20
	}

	const query = `
		SELECT
			id,
			conversation_id,
			role,
			content,
			token_count,
			created_at
		FROM messages
		WHERE conversation_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := r.db.QueryContext(
		ctx,
		query,
		conversationID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}

	defer rows.Close()

	messages := make([]*models.Message, 0, limit)

	for rows.Next() {

		var message models.Message

		if err := rows.Scan(
			&message.ID,
			&message.ConversationID,
			&message.Role,
			&message.Content,
			&message.TokenCount,
			&message.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf(
				"scan message: %w",
				err,
			)
		}

		messages = append(
			messages,
			&message,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate messages: %w",
			err,
		)
	}

	// We queried newest first for efficiency,
	// but the LLM should receive history chronologically.
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}
