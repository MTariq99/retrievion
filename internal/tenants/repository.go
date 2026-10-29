package tenants

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
)

type TenantRepository struct {
	db *sql.DB
}

type OutboxMessage struct {
	ID       string
	Payload  []byte
	Attempts int
}

func NewTanantRepository(db *sql.DB) *TenantRepository {
	return &TenantRepository{db: db}
}

func (tr *TenantRepository) CreateTenantInvitations(ctx context.Context, tenantID, invitedBy uuid.UUID, invites []models.NewInvitation, expiresAt time.Time, outbox []models.OutboxMessage) error {
	if tenantID == uuid.Nil {
		return errs.ErrTenantIDRequired
	}
	if invitedBy == uuid.Nil {
		return errs.ErrIDRequired
	}
	if len(invites) == 0 {
		return errs.ErrInvalidInvitation
	}

	tx, err := tr.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, inv := range invites {
		if _, err := tx.ExecContext(
			ctx, `
			UPDATE tenant_invitations
			SET status = $3
			WHERE tenant_id = $1
			  AND lower(email) = $2
			  AND status = $4
			  AND expires_at < NOW()`,
			tenantID,
			inv.Email,
			consts.InvitationStatusRevoked,
			consts.InvitationStatusPending,
		); err != nil {
			return fmt.Errorf("expire stale invitation: %w", err)
		}

		if _, err := tx.ExecContext(
			ctx, `
			INSERT INTO tenant_invitations
				(tenant_id, invited_by_user_id, email, role, token_hash, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			tenantID, invitedBy, inv.Email, inv.Role, inv.TokenHash, expiresAt.UTC(),
		); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return fmt.Errorf("%s: %w", inv.Email, errs.ErrInvitationAlreadyPending)
			}
			return fmt.Errorf("insert invitation for %s: %w", inv.Email, err)
		}
	}

	for _, msg := range outbox {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO outbox_messages (id, kind, payload) VALUES ($1, $2, $3)`,
			msg.ID, msg.Kind, msg.Payload,
		); err != nil {
			return fmt.Errorf("insert outbox message %s: %w", msg.ID, err)
		}
	}

	return tx.Commit()
}

func (tr *TenantRepository) BeginOutboxTx(ctx context.Context) (*TenantOutboxTx, error) {
	tx, err := tr.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin outbox tx: %w", err)
	}

	return &TenantOutboxTx{tx: tx}, nil
}

func (d *TenantOutboxTx) Claim(ctx context.Context, limit int) ([]OutboxMessage, error) {
	rows, err := d.tx.QueryContext(ctx, `
		SELECT id, payload, attempts
		FROM outbox_messages
		WHERE next_attempt_at <= now()
		ORDER BY created_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []OutboxMessage

	for rows.Next() {
		var message OutboxMessage

		if err := rows.Scan(
			&message.ID,
			&message.Payload,
			&message.Attempts,
		); err != nil {
			return nil, err
		}

		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

func (d *TenantOutboxTx) RecordFailure(ctx context.Context, id string, message string, retryAfter time.Duration) error {
	_, err := d.tx.ExecContext(ctx, `
		UPDATE outbox_messages
		SET attempts = attempts + 1,
		    last_error = $2,
		    next_attempt_at = now() + $3 * interval '1 second'
		WHERE id = $1
	`, id, message, retryAfter.Seconds())
	if err != nil {
		return fmt.Errorf("record outbox failure %s: %w", id, err)
	}

	return nil
}

func (d *TenantOutboxTx) Delete(ctx context.Context, id string) error {
	_, err := d.tx.ExecContext(ctx, `
		DELETE FROM outbox_messages
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("delete outbox message %s: %w", id, err)
	}

	return nil
}

func (d *TenantOutboxTx) Commit() error {
	return d.tx.Commit()
}

func (d *TenantOutboxTx) Rollback() error {
	return d.tx.Rollback()
}

type TenantOutboxTx struct {
	tx *sql.Tx
}
