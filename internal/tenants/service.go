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

	"github.com/mtariq99/retrievion/internal/auth"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/internal/notifications"
	"github.com/mtariq99/retrievion/models"
	"github.com/mtariq99/retrievion/utils"
)

type userReader interface {
	FindUserByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	FindUserByEmail(ctx context.Context, email string) (*models.User, error)
	GetMembership(ctx context.Context, tenantID, userID uuid.UUID) (*models.TenantMember, error)
}
type InvitationPublisher interface {
	PublishInvitation(ctx context.Context, messageID string, body []byte) error
}
type TenantMutation interface {
	CreateTenantInvitations(ctx context.Context, tenantID, invitedBy uuid.UUID, invites []models.NewInvitation, expiresAt time.Time) error
}
type TenantService struct {
	userReader    userReader
	TenantMutator TenantMutation
	emailService  *notifications.EmailService
	Cfg           *models.Config
	publisher     InvitationPublisher
}

func NewTenantService(cfg *models.Config, userReader *auth.Repository, repo *TenantRepository, es *notifications.EmailService, publisher InvitationPublisher) *TenantService {
	return &TenantService{
		userReader:    userReader,
		TenantMutator: repo,
		emailService:  es,
		Cfg:           cfg,
		publisher:     publisher,
	}
}

func (ts *TenantService) InviteUsers(ctx context.Context, ownerTenantID, userID uuid.UUID, req *models.InviteUsers) error {
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
		return err
	}
	if membership.Role != models.RoleOwner {
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
		role := strings.TrimSpace(u.Role)

		switch role {
		case "admin", "member", "viewer":
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
	for i, inv := range invites {
		rows[i] = models.NewInvitation{Email: inv.Email, Role: inv.Role, TokenHash: inv.Hash}
	}
	if err := ts.TenantMutator.CreateTenantInvitations(ctx, ownerTenantID, userID, rows, expiresAt); err != nil {
		return fmt.Errorf("create invitations: %w", err)
	}
	var publishErrs []error

	for _, inv := range invites {
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
			publishErrs = append(publishErrs, fmt.Errorf("encode invitation for %s: %w", inv.Email, err))
			continue
		}
		if err := ts.publisher.PublishInvitation(ctx, msg.MessageID, body); err != nil {
			publishErrs = append(publishErrs, fmt.Errorf("queue invitation for %s: %w", inv.Email, err))
		}
	}

	return errors.Join(publishErrs...)
}
