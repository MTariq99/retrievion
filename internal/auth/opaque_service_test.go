package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mtariq99/retrievion/internal/enums"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
)

type testOpaqueStore struct {
	tokens   map[string]*models.OpaqueToken
	mu       sync.Mutex
	consumed map[string]time.Time
}

func newTestOpaqueStore() *testOpaqueStore {
	return &testOpaqueStore{tokens: make(map[string]*models.OpaqueToken)}
}

func (s *testOpaqueStore) SaveToken(_ context.Context, token *models.OpaqueToken) error {
	copied := *token
	s.tokens[token.TokenID] = &copied
	return nil
}

func (s *testOpaqueStore) GetToken(_ context.Context, id string) (*models.OpaqueToken, error) {
	token, ok := s.tokens[id]
	if !ok {
		return nil, errors.New("token not found")
	}
	copied := *token
	return &copied, nil
}

func (s *testOpaqueStore) RevokeToken(_ context.Context, id string) error {
	token, ok := s.tokens[id]
	if !ok {
		return errors.New("token not found")
	}
	token.Revoked = true
	return nil
}

func (s *testOpaqueStore) RevokeAllUserTokens(_ context.Context, userID uuid.UUID) error {
	for _, token := range s.tokens {
		if token.UserID == userID {
			token.Revoked = true
		}
	}
	return nil
}

type testTenantMembershipReader struct {
	membership *models.TenantMember
	err        error
	calls      int
}

func (r *testTenantMembershipReader) GetMembership(
	_ context.Context,
	tenantID, userID uuid.UUID,
) (*models.TenantMember, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return r.membership, nil
}

func newTestOpaqueService(
	store *testOpaqueStore,
	membership *testTenantMembershipReader,
) *OpaqueTokenService {
	return NewOpaqueTokenService(store, NewTokenConfig(), membership)
}

func TestIntrospectToken_PreservesServiceIdentity(t *testing.T) {
	store := newTestOpaqueStore()
	store.tokens["stok_test"] = &models.OpaqueToken{
		TokenID:   "stok_test",
		TokenType: "service",
		IsService: true,
		ClientID:  "client-123",
		TenantID:  uuid.New(),
		ExpiresAt: time.Now().Add(time.Minute),
	}

	service := newTestOpaqueService(store, &testTenantMembershipReader{})
	got, err := service.IntrospectToken(context.Background(), "stok_test")
	if err != nil {
		t.Fatal(err)
	}

	if !got.Active || !got.IsService || got.ClientID != "client-123" {
		t.Fatalf("service identity not preserved: %+v", got)
	}
}

func TestValidateUserTenantMembership(t *testing.T) {
	tenantID, userID := uuid.New(), uuid.New()

	tests := []struct {
		name     string
		reader   *testTenantMembershipReader
		tenantID uuid.UUID
		userID   uuid.UUID
		wantErr  bool
	}{
		{
			name: "valid membership",
			reader: &testTenantMembershipReader{
				membership: &models.TenantMember{
					TenantID: tenantID,
					UserID:   userID,
				},
			},
			tenantID: tenantID,
			userID:   userID,
		},
		{
			name: "membership removed",
			reader: &testTenantMembershipReader{
				err: errs.ErrNotFound,
			},
			tenantID: tenantID,
			userID:   userID,
			wantErr:  true,
		},
		{
			name: "membership lookup failure",
			reader: &testTenantMembershipReader{
				err: errors.New("database unavailable"),
			},
			tenantID: tenantID,
			userID:   userID,
			wantErr:  true,
		},
		{
			name: "mismatched membership identity",
			reader: &testTenantMembershipReader{
				membership: &models.TenantMember{
					TenantID: uuid.New(),
					UserID:   userID,
				},
			},
			tenantID: tenantID,
			userID:   userID,
			wantErr:  true,
		},
		{
			name:     "missing tenant ID",
			reader:   &testTenantMembershipReader{},
			tenantID: uuid.Nil,
			userID:   userID,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newTestOpaqueService(
				newTestOpaqueStore(),
				tt.reader,
			)

			err := service.ValidateUserTenantMembership(
				context.Background(),
				tt.tenantID,
				tt.userID,
			)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRefreshAccessToken_RejectsRemovedMembership(t *testing.T) {
	store := newTestOpaqueStore()
	tenantID, userID := uuid.New(), uuid.New()

	store.tokens["rtok_test"] = &models.OpaqueToken{
		TokenID:   "rtok_test",
		UserID:    userID,
		TenantID:  tenantID,
		UserRole:  string(enums.RoleUser),
		TokenType: "refresh",
		ExpiresAt: time.Now().Add(time.Hour),
	}

	reader := &testTenantMembershipReader{err: errs.ErrNotFound}
	service := newTestOpaqueService(store, reader)

	_, err := service.RefreshAccessToken(
		context.Background(),
		"rtok_test",
		"127.0.0.1",
		"test-agent",
	)
	if err == nil {
		t.Fatal("expected refresh to fail after membership removal")
	}

	if _, exists := store.tokens["rtok_test"]; !exists {
		t.Fatal("original refresh token should remain stored")
	}

	if len(store.tokens) != 1 {
		t.Fatalf("expected no new tokens, got %d stored tokens", len(store.tokens))
	}
}

func TestRefreshAccessToken_RejectsServiceToken(t *testing.T) {
	store := newTestOpaqueStore()
	store.tokens["stok_test"] = &models.OpaqueToken{
		TokenID:   "stok_test",
		TokenType: "refresh",
		IsService: true,
		ClientID:  "client-123",
		TenantID:  uuid.New(),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	service := newTestOpaqueService(store, &testTenantMembershipReader{})

	_, err := service.RefreshAccessToken(
		context.Background(),
		"stok_test",
		"",
		"",
	)
	if err == nil {
		t.Fatal("expected service token to be rejected by user refresh flow")
	}
}

// add a consumed map to each stub struct and initialise it in its constructor
//   consumed map[string]bool

func (s *testOpaqueStore) ConsumeToken(ctx context.Context, tokenID string) (*models.OpaqueToken, error) {
	s.mu.Lock() // drop the locking if your stub doesn't have a mutex
	defer s.mu.Unlock()

	tok, ok := s.tokens[tokenID]
	if !ok || time.Now().After(tok.ExpiresAt) {
		return nil, errs.ErrTokenNotFound
	}
	if s.consumed == nil {
		s.consumed = make(map[string]time.Time)
	}
	// ...
	if _, done := s.consumed[tokenID]; done {
		return nil, errs.ErrTokenAlreadyConsumed
	}
	s.consumed[tokenID] = time.Now()

	cp := *tok // return a copy, like the Redis version does
	return &cp, nil
}

func (s *testOpaqueStore) ReleaseToken(ctx context.Context, tokenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.consumed, tokenID)
	return nil
}

func (s *testOpaqueStore) ConsumedAt(ctx context.Context, tokenID string) (time.Time, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.consumed[tokenID]
	return t, ok, nil
}
