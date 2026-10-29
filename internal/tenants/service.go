package tenants

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
	"github.com/mtariq99/retrievion/utils"
)

type UserReader interface {
	FindUserByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	FindUserByEmail(ctx context.Context, email string) (*models.User, error)
	GetMembership(ctx context.Context, tenantID, userID uuid.UUID) (*models.TenantMember, error)
}
type InvitationPublisher interface {
	PublishInvitation(ctx context.Context, messageID string, body []byte) error
}
type TenantMutation interface {
	CreateTenantInvitations(ctx context.Context, tenantID, invitedBy uuid.UUID, invites []models.NewInvitation, expiresAt time.Time, outbox []models.OutboxMessage) error
}
type TenantService struct {
	userReader    UserReader
	TenantMutator TenantMutation
	Cfg           *models.Config
	publisher     InvitationPublisher
}

func NewTenantService(cfg *models.Config, userReader UserReader, repo *TenantRepository, publisher InvitationPublisher) *TenantService {
	return &TenantService{
		userReader:    userReader,
		TenantMutator: repo,
		Cfg:           cfg,
		publisher:     publisher,
	}
}

func (ts *TenantService) InviteUsers(
	ctx context.Context,
	ownerTenantID, userID uuid.UUID,
	req *models.InviteUsers,
) error {
	if ownerTenantID == uuid.Nil {
		return errs.ErrTenantIDRequired
	}
	if userID == uuid.Nil {
		return errs.ErrIDRequired
	}
	if req == nil || len(req.Users) == 0 {
		return errs.ErrInvalidInvitation
	}

	membership, err := ts.userReader.GetMembership(ctx, ownerTenantID, userID)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return errs.ErrUnauthorized
		}
		return fmt.Errorf("verify tenant membership: %w", err)
	}

	if membership == nil ||
		membership.TenantID != ownerTenantID ||
		membership.UserID != userID ||
		membership.Role != consts.RoleOwner {
		return errs.ErrUnauthorized
	}

	seen := make(map[string]struct{}, len(req.Users))
	invites := make([]models.PendingInvite, 0, len(req.Users))

	for _, u := range req.Users {
		addr, err := mail.ParseAddress(strings.TrimSpace(u.Email))
		if err != nil {
			return errs.ErrInvalidInvitation
		}

		email := strings.ToLower(addr.Address)
		role := consts.Role(strings.TrimSpace(u.Role))

		switch role {
		case consts.RoleAdmin, consts.RoleMember, consts.RoleViewer:
		default:
			return errs.ErrInvalidInvitation
		}

		if _, dup := seen[email]; dup {
			return errs.ErrInvalidInvitation
		}
		seen[email] = struct{}{}

		token, err := utils.GenerateToken()
		if err != nil {
			return fmt.Errorf("generate invitation token: %w", err)
		}

		invites = append(invites, models.PendingInvite{
			Name:  u.Name,
			Email: email,
			Role:  role,
			Token: token,
			Hash:  utils.HashTokenBytes(token),
		})
	}

	expiresAt := time.Now().UTC().Add(consts.InvitationTTL)

	rows := make([]models.NewInvitation, len(invites))
	msgs := make([]models.OutboxMessage, len(invites))

	for i, inv := range invites {
		rows[i] = models.NewInvitation{
			Email:     inv.Email,
			Role:      inv.Role,
			TokenHash: inv.Hash,
		}

		msg := models.InvitationEmailMessage{
			MessageID:     uuid.NewString(),
			TenantID:      ownerTenantID,
			Email:         inv.Email,
			Role:          inv.Role,
			WorkspaceName: membership.TenantName,
			Token:         inv.Token,
			CreatedAt:     time.Now().UTC(),
		}

		body, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("encode invitation for %s: %w", inv.Email, err)
		}

		msgs[i] = models.OutboxMessage{
			ID:      msg.MessageID,
			Kind:    "invitation.email",
			Payload: body,
		}
	}

	if err := ts.TenantMutator.CreateTenantInvitations(ctx, ownerTenantID, userID, rows, expiresAt, msgs); err != nil {
		return fmt.Errorf("create invitations: %w", err)
	}

	return nil
}
