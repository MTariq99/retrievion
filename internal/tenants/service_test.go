package tenants

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
)

type invitationTestUserReader struct {
	membership *models.TenantMember
	err        error
}

func (f *invitationTestUserReader) FindUserByID(
	context.Context, uuid.UUID,
) (*models.User, error) {
	return nil, nil
}

func (f *invitationTestUserReader) FindUserByEmail(
	context.Context, string,
) (*models.User, error) {
	return nil, nil
}

func (f *invitationTestUserReader) GetMembership(
	context.Context, uuid.UUID, uuid.UUID,
) (*models.TenantMember, error) {
	return f.membership, f.err
}

type invitationTestMutator struct {
	calls  int
	err    error
	outbox []models.OutboxMessage
}

func (f *invitationTestMutator) CreateTenantInvitations(
	ctx context.Context,
	tenantID, invitedBy uuid.UUID,
	invites []models.NewInvitation,
	expiresAt time.Time,
	outbox []models.OutboxMessage,
) error {
	f.calls++
	f.outbox = append(f.outbox, outbox...)
	return f.err
}

func (f *invitationTestMutator) AcceptInvitation(context.Context, uuid.UUID, string, []byte) error {
	return nil
}

type invitationTestPublisher struct {
	calls int
	err   error
}

func (f *invitationTestPublisher) PublishInvitation(
	context.Context, string, []byte,
) error {
	f.calls++
	return f.err
}

func TestInviteUsersAuthorization(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name          string
		membership    *models.TenantMember
		membershipErr error
		wantErr       error
	}{
		{
			name: "owner is authorized",
			membership: &models.TenantMember{
				TenantID:   tenantID,
				UserID:     userID,
				Role:       consts.RoleOwner,
				TenantName: "Test workspace",
			},
		},
		{
			name: "non-owner is denied",
			membership: &models.TenantMember{
				TenantID: tenantID,
				UserID:   userID,
				Role:     consts.RoleMember,
			},
			wantErr: errs.ErrUnauthorized,
		},
		{
			name:          "missing membership is denied",
			membershipErr: errs.ErrNotFound,
			wantErr:       errs.ErrUnauthorized,
		},
		{
			name:    "nil membership is denied",
			wantErr: errs.ErrUnauthorized,
		},
		{
			name: "membership for another tenant is denied",
			membership: &models.TenantMember{
				TenantID: uuid.New(),
				UserID:   userID,
				Role:     consts.RoleOwner,
			},
			wantErr: errs.ErrUnauthorized,
		},
		{
			name: "membership for another user is denied",
			membership: &models.TenantMember{
				TenantID: tenantID,
				UserID:   uuid.New(),
				Role:     consts.RoleOwner,
			},
			wantErr: errs.ErrUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &invitationTestUserReader{
				membership: tt.membership,
				err:        tt.membershipErr,
			}
			mutator := &invitationTestMutator{}
			publisher := &invitationTestPublisher{}

			service := &TenantService{
				userReader:    reader,
				TenantMutator: mutator,
				publisher:     publisher,
			}

			err := service.InviteUsers(
				context.Background(),
				tenantID,
				userID,
				&models.InviteUsers{
					Users: []models.InviteUser{{
						Name:  "Invitee",
						Email: "invitee@example.com",
						Role:  string(consts.RoleMember),
					}},
				},
			)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				if mutator.calls != 0 {
					t.Errorf("unauthorized request wrote invitations")
				}
				if publisher.calls != 0 {
					t.Errorf("unauthorized request published invitations")
				}
				return
			}

			if err != nil {
				t.Fatalf("expected success, got %v", err)
			}
			if mutator.calls != 1 {
				t.Fatalf("expected one invitation write, got %d", mutator.calls)
			}

			if len(mutator.outbox) != 1 {
				t.Fatalf("expected one outbox message, got %d", len(mutator.outbox))
			}

			if mutator.outbox[0].Kind != "invitation.email" {
				t.Errorf("expected outbox kind %q, got %q",
					"invitation.email", mutator.outbox[0].Kind)
			}

			if len(mutator.outbox[0].Payload) == 0 {
				t.Error("expected non-empty outbox payload")
			}

			if publisher.calls != 0 {
				t.Errorf("expected no direct publication, got %d", publisher.calls)
			}
		})
	}
}

func TestInviteUsersMembershipRepositoryErrorFailsClosed(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	repositoryErr := errors.New("database unavailable")

	mutator := &invitationTestMutator{}
	publisher := &invitationTestPublisher{}

	service := &TenantService{
		userReader: &invitationTestUserReader{
			err: repositoryErr,
		},
		TenantMutator: mutator,
		publisher:     publisher,
	}

	err := service.InviteUsers(
		context.Background(),
		tenantID,
		userID,
		&models.InviteUsers{
			Users: []models.InviteUser{{
				Name:  "Invitee",
				Email: "invitee@example.com",
				Role:  string(consts.RoleMember),
			}},
		},
	)

	if !errors.Is(err, repositoryErr) {
		t.Fatalf("expected wrapped repository error, got %v", err)
	}
	if mutator.calls != 0 {
		t.Errorf("repository error must not write invitations")
	}
	if publisher.calls != 0 {
		t.Errorf("repository error must not publish invitations")
	}
}

// Ensure the fake reader satisfies the service's intended dependency contract.
var _ interface {
	FindUserByID(context.Context, uuid.UUID) (*models.User, error)
	FindUserByEmail(context.Context, string) (*models.User, error)
	GetMembership(context.Context, uuid.UUID, uuid.UUID) (*models.TenantMember, error)
} = (*invitationTestUserReader)(nil)
