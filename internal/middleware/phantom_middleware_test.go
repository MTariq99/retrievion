package middleware

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mtariq99/retrievion/internal/auth"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
)

type middlewareTestTokenStore struct {
	tokens   map[string]*models.OpaqueToken
	mu       sync.Mutex
	consumed map[string]time.Time
}

func newMiddlewareTestTokenStore(tokens ...*models.OpaqueToken) *middlewareTestTokenStore {
	store := &middlewareTestTokenStore{
		tokens: make(map[string]*models.OpaqueToken),
	}
	for _, token := range tokens {
		copied := *token
		store.tokens[token.TokenID] = &copied
	}
	return store
}

func (s *middlewareTestTokenStore) SaveToken(
	_ context.Context,
	token *models.OpaqueToken,
) error {
	copied := *token
	s.tokens[token.TokenID] = &copied
	return nil
}

func (s *middlewareTestTokenStore) GetToken(
	_ context.Context,
	tokenID string,
) (*models.OpaqueToken, error) {
	token, ok := s.tokens[tokenID]
	if !ok {
		return nil, errors.New("token not found")
	}
	copied := *token
	return &copied, nil
}

func (s *middlewareTestTokenStore) RevokeToken(
	_ context.Context,
	tokenID string,
) error {
	token, ok := s.tokens[tokenID]
	if !ok {
		return errors.New("token not found")
	}
	token.Revoked = true
	return nil
}

func (s *middlewareTestTokenStore) RevokeAllUserTokens(
	_ context.Context,
	userID uuid.UUID,
) error {
	for _, token := range s.tokens {
		if token.UserID == userID {
			token.Revoked = true
		}
	}
	return nil
}

type middlewareTestMembershipReader struct {
	membership *models.TenantMember
	err        error
	calls      int
}

func (r *middlewareTestMembershipReader) GetMembership(
	_ context.Context,
	tenantID uuid.UUID,
	userID uuid.UUID,
) (*models.TenantMember, error) {
	r.calls++

	if r.err != nil {
		return nil, r.err
	}
	if r.membership == nil {
		return nil, errors.New("membership not found")
	}
	if r.membership.TenantID != tenantID || r.membership.UserID != userID {
		return nil, errors.New("membership identity mismatch")
	}
	return r.membership, nil
}

func runPhantomMiddlewareTest(
	t *testing.T,
	token *models.OpaqueToken,
	membership *middlewareTestMembershipReader,
) int {
	t.Helper()

	store := newMiddlewareTestTokenStore(token)
	opaqueService := auth.NewOpaqueTokenService(
		store,
		auth.NewTokenConfig(),
		membership,
	)

	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	middleware := &PhantomTokenMiddleware{
		opaqueService: opaqueService,
		jwtPrivateKey: privateKey,
		issuer:        "middleware-test",
		logger:        zap.NewNop(),
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ExchangeOpaqueForJWT())
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+token.TokenID)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)
	return recorder.Code
}

func TestPhantomTokenMiddleware_TenantIsolation(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()

	validUserToken := &models.OpaqueToken{
		TokenID:   "tok_valid_user",
		UserID:    userID,
		TenantID:  tenantID,
		TokenType: "access",
		ExpiresAt: time.Now().Add(time.Minute),
	}

	tests := []struct {
		name                string
		token               *models.OpaqueToken
		membership          *middlewareTestMembershipReader
		wantStatus          int
		wantMembershipCalls int
	}{
		{
			name:  "valid user access token with membership",
			token: validUserToken,
			membership: &middlewareTestMembershipReader{
				membership: &models.TenantMember{
					TenantID: tenantID,
					UserID:   userID,
				},
			},
			wantStatus:          http.StatusNoContent,
			wantMembershipCalls: 1,
		},
		{
			name: "refresh token cannot authenticate a request",
			token: &models.OpaqueToken{
				TokenID:   "rtok_refresh",
				UserID:    userID,
				TenantID:  tenantID,
				TokenType: "refresh",
				ExpiresAt: time.Now().Add(time.Minute),
			},
			membership:          &middlewareTestMembershipReader{},
			wantStatus:          http.StatusUnauthorized,
			wantMembershipCalls: 0,
		},
		{
			name:                "removed tenant membership is rejected",
			token:               validUserToken,
			membership:          &middlewareTestMembershipReader{},
			wantStatus:          http.StatusUnauthorized,
			wantMembershipCalls: 1,
		},
		{
			name:  "membership database failure fails closed",
			token: validUserToken,
			membership: &middlewareTestMembershipReader{
				err: errors.New("database unavailable"),
			},
			wantStatus:          http.StatusUnauthorized,
			wantMembershipCalls: 1,
		},
		{
			name: "valid service token does not require user membership",
			token: &models.OpaqueToken{
				TokenID:   "stok_valid",
				TenantID:  tenantID,
				TokenType: "service",
				IsService: true,
				ClientID:  "client-123",
				ExpiresAt: time.Now().Add(time.Minute),
			},
			membership:          &middlewareTestMembershipReader{},
			wantStatus:          http.StatusNoContent,
			wantMembershipCalls: 0,
		},
		{
			name: "service token without client ID is rejected",
			token: &models.OpaqueToken{
				TokenID:   "stok_missing_client",
				TenantID:  tenantID,
				TokenType: "service",
				IsService: true,
				ExpiresAt: time.Now().Add(time.Minute),
			},
			membership:          &middlewareTestMembershipReader{},
			wantStatus:          http.StatusUnauthorized,
			wantMembershipCalls: 0,
		},
		{
			name: "service identity with wrong token type is rejected",
			token: &models.OpaqueToken{
				TokenID:   "stok_wrong_type",
				TenantID:  tenantID,
				TokenType: "access",
				IsService: true,
				ClientID:  "client-123",
				ExpiresAt: time.Now().Add(time.Minute),
			},
			membership:          &middlewareTestMembershipReader{},
			wantStatus:          http.StatusUnauthorized,
			wantMembershipCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := runPhantomMiddlewareTest(t, tt.token, tt.membership)

			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d", status, tt.wantStatus)
			}
			if tt.membership.calls != tt.wantMembershipCalls {
				t.Fatalf(
					"membership calls = %d, want %d",
					tt.membership.calls,
					tt.wantMembershipCalls,
				)
			}
		})
	}
}

func runUserRouteMiddlewareTest(
	t *testing.T,
	token *models.OpaqueToken,
	membership *middlewareTestMembershipReader,
) int {
	t.Helper()

	store := newMiddlewareTestTokenStore(token)
	opaqueService := auth.NewOpaqueTokenService(
		store,
		auth.NewTokenConfig(),
		membership,
	)

	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	phantom := &PhantomTokenMiddleware{
		opaqueService: opaqueService,
		jwtPrivateKey: privateKey,
		issuer:        "middleware-test",
		logger:        zap.NewNop(),
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(phantom.ExchangeOpaqueForJWT())
	router.Use(RequireUserToken())
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+token.TokenID)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)
	return recorder.Code
}

func TestRequireUserToken_AfterPhantomExchange(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name       string
		token      *models.OpaqueToken
		membership *middlewareTestMembershipReader
		wantStatus int
	}{
		{
			name: "valid user token is allowed",
			token: &models.OpaqueToken{
				TokenID:   "tok_user_route",
				UserID:    userID,
				TenantID:  tenantID,
				TokenType: "access",
				ExpiresAt: time.Now().Add(time.Minute),
			},
			membership: &middlewareTestMembershipReader{
				membership: &models.TenantMember{
					TenantID: tenantID,
					UserID:   userID,
				},
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name: "valid service token is rejected on user route",
			token: &models.OpaqueToken{
				TokenID:   "stok_user_route",
				TenantID:  tenantID,
				TokenType: "service",
				IsService: true,
				ClientID:  "client-123",
				ExpiresAt: time.Now().Add(time.Minute),
			},
			membership: &middlewareTestMembershipReader{},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := runUserRouteMiddlewareTest(
				t,
				tt.token,
				tt.membership,
			)

			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d", status, tt.wantStatus)
			}
		})
	}
}

func (s *middlewareTestTokenStore) ConsumeToken(ctx context.Context, tokenID string) (*models.OpaqueToken, error) {
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

func (s *middlewareTestTokenStore) ReleaseToken(ctx context.Context, tokenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.consumed, tokenID)
	return nil
}

func (s *middlewareTestTokenStore) ConsumedAt(ctx context.Context, tokenID string) (time.Time, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.consumed[tokenID]
	return t, ok, nil
}
