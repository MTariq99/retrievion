package chat

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

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

func (r *Repository) CreateMessage(ctx context.Context, conversationID uuid.UUID, role string, content string) (*models.Message, error) {
	if conversationID == uuid.Nil {
		return nil, fmt.Errorf("conversation ID is required")
	}

	if role == "" {
		return nil, fmt.Errorf("message role is required")
	}

	if content == "" {
		return nil, fmt.Errorf("message content is required")
	}

	const query = `
		INSERT INTO messages (
			id,
			conversation_id,
			role,
			content,
			created_at
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			NOW()
		)
		RETURNING
			id,
			conversation_id,
			role,
			content,
			token_count,
			created_at
	`

	messageID := uuid.New()

	var message models.Message

	err := r.db.QueryRowContext(
		ctx,
		query,
		messageID,
		conversationID,
		role,
		content,
	).Scan(
		&message.ID,
		&message.ConversationID,
		&message.Role,
		&message.Content,
		&message.TokenCount,
		&message.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}

	return &message, nil
}

func (r *Repository) CreateMessageSources(ctx context.Context, messageID uuid.UUID, sources []models.MessageSource) error {
	if messageID == uuid.Nil {
		return fmt.Errorf("message ID is required")
	}

	if len(sources) == 0 {
		return nil
	}

	const query = `
		INSERT INTO message_sources (
			id,
			message_id,
			chunk_id,
			document_id,
			citation_index,
			created_at
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			NOW()
		)
	`

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		_ = tx.Rollback()
	}()

	for _, source := range sources {

		_, err := tx.ExecContext(
			ctx,
			query,
			uuid.New(),
			messageID,
			source.ChunkID,
			source.DocumentID,
			source.CitationIndex,
		)
		if err != nil {
			return fmt.Errorf(
				"create message source: %w",
				err,
			)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf(
			"commit message sources: %w",
			err,
		)
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
