package tenants

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
)

type TenantRepository struct {
	db *sql.DB
}

func NewTanantRepository(db *sql.DB) *TenantRepository {
	return &TenantRepository{db: db}
}

func (tr *TenantRepository) CreateTenantInvitations(ctx context.Context, tenantID, invitedBy uuid.UUID, invites []models.NewInvitation, expiresAt time.Time) error {
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
		if _, err := tx.ExecContext(ctx, `
			UPDATE tenant_invitations
			SET status = 'revoked'
			WHERE tenant_id = $1
			  AND lower(email) = $2
			  AND status = 'pending'
			  AND expires_at < NOW()`,
			tenantID, inv.Email); err != nil {
			return fmt.Errorf("expire stale invitation: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tenant_invitations
				(tenant_id, invited_by_user_id, email, role, token_hash, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			tenantID, invitedBy, inv.Email, inv.Role, inv.TokenHash, expiresAt.UTC()); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return fmt.Errorf("%s: %w", inv.Email, errs.ErrInvitationAlreadyPending)
			}
			return fmt.Errorf("insert invitation for %s: %w", inv.Email, err)
		}
	}

	return tx.Commit()
}
