package notifications

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/consts"
)

type NotificationRepository struct {
	db *sql.DB
}

func NewNotificationRepository(db *sql.DB) *NotificationRepository {
	return &NotificationRepository{
		db: db,
	}
}

func (nr *NotificationRepository) FindPendingInvitation(ctx context.Context, tokenHash []byte) (uuid.UUID, error) {
	var id uuid.UUID
	lookupTenantInvitations := `
	SELECT id 
	FROM tenant_invitations
	WHERE token_hash = $1 AND status = $2 AND expires_at > now()
	`
	err := nr.db.QueryRowContext(ctx, lookupTenantInvitations, tokenHash, consts.InvitationStatusPending).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("find pending invitation: %w", err)
	}
	return id, nil
}

func (nr *NotificationRepository) Claim(ctx context.Context, messageID, invitationID uuid.UUID, reclaimAfter time.Duration) (consts.DeliveryState, error) {
	var claimed uuid.UUID
	insertEmailNotificationLog := `
	INSERT INTO email_notification_log(
	message_id,
	invitation_id,
	status
	)
	VALUES
	($1,$2,'sending')
	ON CONFLICT(message_id) DO UPDATE
	set status = 'sending',updated_at=now()
	WHERE email_invitation_log.status = 'failed' OR (email_invitation_log.status = 'sending' AND email_invitation_log.updated_at < now()-make_interval(secs=>$3))
	RETURNING message_id
	`
	err := nr.db.QueryRowContext(ctx, insertEmailNotificationLog, messageID, invitationID, reclaimAfter).Scan(&claimed)
	if err == nil {
		return consts.DeliveryClaimed, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("claim delivery: %w", err)
	}
	var status string
	emailInvitationLogLookup := `SELECT status FROM email_invitation_log WHERE message_id = $1`
	if err := nr.db.QueryRowContext(ctx, emailInvitationLogLookup, messageID).Scan(&status); err != nil {
		return 0, fmt.Errorf("read delivery status: %w", err)
	}
	if status == "sent" {
		return consts.DeliveryAlreadySent, nil
	}
	return consts.DeliveryInProgress, nil
}

func (nr *NotificationRepository) MarkSent(ctx context.Context, messageID uuid.UUID) error {
	return nr.setStatus(ctx, messageID, "sent")
}

func (nr *NotificationRepository) MarkFailed(ctx context.Context, messageID uuid.UUID) error {
	return nr.setStatus(ctx, messageID, "failed")
}

func (nr *NotificationRepository) setStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := nr.db.ExecContext(ctx,
		`UPDATE email_invitation_log SET status = $2, updated_at = now() WHERE message_id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("set delivery status %q: %w", status, err)
	}
	return nil
}
