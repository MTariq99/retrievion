package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/internal/storage"
	"github.com/mtariq99/retrievion/models"
	"github.com/mtariq99/retrievion/utils"
	"go.uber.org/zap"
)

type retryConsumer interface {
	ConsumeWithRetry(ctx context.Context, opts storage.RetryOptions, handler func(ctx context.Context, body []byte) error) error
}
type DeliveryStore interface {
	FindPendingInvitation(ctx context.Context, tokenHash []byte) (uuid.UUID, error)
	Claim(ctx context.Context, messageID, invitationID uuid.UUID, reclaimAfter time.Duration) (consts.DeliveryState, error)
	MarkSent(ctx context.Context, messageID uuid.UUID) error
	MarkFailed(ctx context.Context, messageID uuid.UUID) error
}

type InvitationWorker struct {
	queue         retryConsumer
	email         *EmailService
	cfg           *models.Config
	logger        *zap.Logger
	DeliveryStore DeliveryStore
}

func NewInvitationWorker(q *storage.RabbitMQQueue, email *EmailService, cfg *models.Config, logger *zap.Logger, ds DeliveryStore) *InvitationWorker {
	return &InvitationWorker{queue: q, email: email, cfg: cfg, logger: logger, DeliveryStore: ds}
}

func (iw *InvitationWorker) Run(ctx context.Context) error {
	iw.logger.Info("invitation worker starting")

	opts := storage.RetryOptions{
		Queue:       consts.InvitationQueue,
		Exchange:    consts.InvitationExchange,
		RetryKey:    consts.InvitationRetryRoutingKey,
		DeadKey:     consts.InvitationDeadRoutingKey,
		MaxAttempts: consts.InvitationMaxAttempts,
		Concurrency: 5,
	}
	return iw.queue.ConsumeWithRetry(ctx, opts, iw.handle)
}

func (iw *InvitationWorker) handle(ctx context.Context, body []byte) error {
	var m models.InvitationEmailMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return fmt.Errorf("%w: unmarshal: %v", storage.ErrPermanent, err)
	}

	messageID, err := utils.ParseUUID(m.MessageID, "message id")
	if err != nil {
		return fmt.Errorf("%w: invalid message_id: %v", storage.ErrPermanent, err)
	}
	invitationID, err := iw.DeliveryStore.FindPendingInvitation(ctx, utils.HashTokenBytes(m.Token))
	if err != nil {
		return err // transient: retry
	}
	if invitationID == uuid.Nil {
		iw.logger.Info("invitation no longer pending, skipping email", zap.String("message_id", m.MessageID))
		return nil
	}
	state, err := iw.DeliveryStore.Claim(ctx, messageID, invitationID, consts.ReclaimAfter)
	if err != nil {
		return err
	}
	switch state {
	case consts.DeliveryAlreadySent:
		iw.logger.Info("duplicate delivery skipped", zap.String("message_id", m.MessageID))
		return nil
	case consts.DeliveryInProgress:
		return errs.ErrDeliveryInProgress
	}
	if err := iw.send(ctx, m); err != nil {
		iw.logger.Warn("invitation email failed", zap.String("message_id", m.MessageID), zap.Error(err))
		iw.finish(messageID, iw.DeliveryStore.MarkFailed)
		return err
	}
	iw.finish(messageID, iw.DeliveryStore.MarkSent)
	iw.logger.Info("invitation email sent", zap.String("message_id", m.MessageID), zap.String("to", m.Email))
	return nil
}

func (iw *InvitationWorker) finish(id uuid.UUID, fn func(context.Context, uuid.UUID) error) {
	ctx, cancel := context.WithTimeout(context.Background(), consts.StatusTimeout)
	defer cancel()
	if err := fn(ctx, id); err != nil {
		iw.logger.Error("failed to record delivery status", zap.String("message_id", id.String()), zap.Error(err))
	}
}

func (iw *InvitationWorker) send(ctx context.Context, m models.InvitationEmailMessage) error {
	base := strings.TrimRight(iw.cfg.FrontEnd.FrontendURL, "/")
	data := models.UserInvitationTemplateData{
		Name:          m.Email,
		Email:         m.Email,
		Title:         "You're invited to join Retrievion",
		Message:       "You've been invited to collaborate on Retrievion. Accept your invitation to get started.",
		WorkspaceName: m.WorkspaceName,
		Role:          m.Role,
		InvitationURL: base + "/accept-invitation?token=" + url.QueryEscape(m.Token),
		Expiry:        "24 hours",
		PrivacyURL:    iw.cfg.FrontEnd.PrivacyURL,
		TermsURL:      iw.cfg.FrontEnd.TermsURL,
		SupportURL:    iw.cfg.FrontEnd.SupportURL,
		Year:          time.Now().Year(),
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- iw.email.SendUserInvitation(data.Email, data)
	}()
	select {
	case err := <-done:
		if err != nil {
			iw.logger.Warn("invitation email failed", zap.String("message_id", m.MessageID), zap.Error(err))
			return err
		}
		iw.logger.Info("invitation email sent", zap.String("message_id", m.MessageID), zap.String("to", m.Email))
		return nil
	case <-ctx.Done():
		return fmt.Errorf("send timed out: %w", ctx.Err())
	}
	return nil
}
