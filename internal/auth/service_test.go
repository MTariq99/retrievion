package auth

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/internal/storage"
	"github.com/mtariq99/retrievion/models"
)

type fakeUsers struct{ u *models.User }

func (f fakeUsers) FindUserByID(_ context.Context, id uuid.UUID) (*models.User, error) {
	if f.u != nil && f.u.ID == id {
		c := *f.u
		return &c, nil
	}
	return nil, errs.ErrNotFound
}

func (f fakeUsers) FindUserByEmail(_ context.Context, e string) (*models.User, error) {
	if f.u != nil && f.u.Email == e {
		c := *f.u
		return &c, nil
	}
	return nil, errs.ErrNotFound
}

type fakeMembers struct{ choices []models.TenantChoice }

func (f *fakeMembers) GetMembership(_ context.Context, t, u uuid.UUID) (*models.TenantMember, error) {
	for _, c := range f.choices {
		if c.TenantID == t {
			return &models.TenantMember{TenantID: t, UserID: u}, nil
		}
	}
	return nil, errs.ErrNotFound
}

func (f *fakeMembers) GetUserTenants(context.Context, uuid.UUID) ([]models.Tenant, error) {
	return nil, nil
}

func (f *fakeMembers) GetUserMemberships(context.Context, uuid.UUID) ([]models.TenantMember, error) {
	return nil, nil
}

func (f *fakeMembers) GetUserTenantChoices(context.Context, uuid.UUID) ([]models.TenantChoice, error) {
	return f.choices, nil
}

type fakeChallenges struct{ m map[string][]byte }

func (f *fakeChallenges) SaveLoginChallenge(_ context.Context, k string, v any, _ time.Duration) error {
	b, _ := json.Marshal(v)
	f.m[k] = b
	return nil
}

func (f *fakeChallenges) ConsumeLoginChallenge(_ context.Context, k string) ([]byte, error) {
	b, ok := f.m[k]
	if !ok {
		return nil, storage.ErrChallengeNotFound
	}
	delete(f.m, k)
	return b, nil
}

type fakeIssuer struct{ lastTenant uuid.UUID }

func (f *fakeIssuer) GenerateTokenPair(_ context.Context, _ *models.User, t uuid.UUID, _, _ string) (*models.TokenPair, error) {
	f.lastTenant = t
	return &models.TokenPair{AccessToken: "tok_a", RefreshToken: "rtok_r", TokenType: "Bearer", ExpiresIn: 900}, nil
}

func newTestService(t *testing.T, tenants ...uuid.UUID) (*Service, *fakeMembers, *fakeIssuer) {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	user := &models.User{ID: uuid.New(), Email: "a@b.com", PasswordHash: string(hash)}
	m := &fakeMembers{}
	for _, id := range tenants {
		m.choices = append(m.choices, models.TenantChoice{TenantID: id, Name: "T"})
	}
	iss := &fakeIssuer{}
	return &Service{
		users: fakeUsers{user}, membership: m, tokens: iss,
		challenges: &fakeChallenges{m: map[string][]byte{}},
	}, m, iss
}

func login(s *Service, pw string) (*models.LoginResponse, error) {
	return s.Login(context.Background(), models.LoginRequest{Email: "A@b.com", Password: pw}, "ip", "ua")
}

func TestLogin_BadCredentialsNeverRevealMemberships(t *testing.T) {
	s, _, _ := newTestService(t) // zero memberships
	if _, err := login(s, "wrong"); err != errs.ErrInvalidCredentials {
		t.Fatalf("got %v", err)
	}
	s.users = fakeUsers{} // unknown email
	if _, err := login(s, "pw"); err != errs.ErrInvalidCredentials {
		t.Fatalf("got %v", err)
	}
}

func TestLogin_ZeroMemberships(t *testing.T) {
	s, _, _ := newTestService(t)
	if _, err := login(s, "pw"); err != errs.ErrNoTenantMembership {
		t.Fatalf("got %v", err)
	}
}

func TestLogin_SingleMembership(t *testing.T) {
	tid := uuid.New()
	s, _, iss := newTestService(t, tid)
	resp, err := login(s, "pw")
	if err != nil || resp.Status != "authenticated" || resp.Token == "" || iss.lastTenant != tid {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

func TestLogin_MultipleMemberships_NoTokens(t *testing.T) {
	s, _, iss := newTestService(t, uuid.New(), uuid.New())
	resp, err := login(s, "pw")
	if err != nil || resp.Status != "tenant_selection_required" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if resp.Token != "" || resp.RefreshToken != "" || iss.lastTenant != uuid.Nil || len(resp.Tenants) != 2 || resp.SelectionToken == "" {
		t.Fatalf("unexpected: %+v", resp)
	}
}

func TestSelectTenant_ReplayAndRevocation(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	s, m, iss := newTestService(t, a, b)
	resp, _ := login(s, "pw")
	req := models.SelectTenantRequest{SelectionToken: resp.SelectionToken, TenantID: b.String()}

	if _, err := s.SelectTenant(context.Background(), req, "ip", "ua"); err != nil || iss.lastTenant != b {
		t.Fatalf("select failed: %v", err)
	}
	if _, err := s.SelectTenant(context.Background(), req, "ip", "ua"); err != errs.ErrInvalidSelectionToken {
		t.Fatalf("replay: got %v", err)
	}

	// Membership revoked between login and selection.
	resp, _ = login(s, "pw")
	m.choices = m.choices[:1] // only tenant a remains
	req = models.SelectTenantRequest{SelectionToken: resp.SelectionToken, TenantID: b.String()}
	if _, err := s.SelectTenant(context.Background(), req, "ip", "ua"); err != errs.ErrTenantAccessDenied {
		t.Fatalf("revoked: got %v", err)
	}
}
