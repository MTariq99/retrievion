package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/enums"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/internal/storage"
	"github.com/mtariq99/retrievion/models"
	"github.com/mtariq99/retrievion/utils"
)

type userReader interface {
	FindUserByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	FindUserByEmail(ctx context.Context, email string) (*models.User, error)
}

type userWriter interface {
	CreateUser(ctx context.Context, u *models.User) error
	CreateUserWithTenant(ctx context.Context, u *models.User, tenantName string) (*models.Tenant, error)
}

type membershipRepo interface {
	GetMembership(ctx context.Context, tenantID, userID uuid.UUID) (*models.TenantMember, error)
	GetUserTenants(ctx context.Context, userID uuid.UUID) ([]models.Tenant, error)
	GetUserMemberships(ctx context.Context, userID uuid.UUID) ([]models.TenantMember, error)
	GetUserTenantChoices(ctx context.Context, userID uuid.UUID) ([]models.TenantChoice, error)
}

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("retrievion-dummy-password"), bcrypt.DefaultCost)

type tokenIssuer interface {
	GenerateTokenPair(ctx context.Context, user *models.User, tenantID uuid.UUID, clientIP, userAgent string) (*models.TokenPair, error)
}

type challengeStore interface {
	SaveLoginChallenge(ctx context.Context, key string, value any, ttl time.Duration) error
	ConsumeLoginChallenge(ctx context.Context, key string) ([]byte, error)
}

type loginChallenge struct {
	UserID   uuid.UUID `json:"user_id"`
	IssuedAt time.Time `json:"issued_at"`
}
type Service struct {
	users      userReader
	userWriter userWriter
	membership membershipRepo
	challenges challengeStore
	tokens     tokenIssuer

	jwtSecret []byte
	jwtTTL    time.Duration
}

func NewService(repo *Repository, cfg *models.Config, opaqueTokens *OpaqueTokenService, redis *storage.RedisClient) *Service {
	return &Service{
		users:      repo,
		userWriter: repo,
		membership: repo,
		challenges: redis,
		tokens:     opaqueTokens,
		jwtSecret:  []byte(cfg.Auth.JWTSecret),
		jwtTTL:     time.Duration(cfg.Auth.JWTExpirationHours) * time.Hour,
	}
}

func (s *Service) hashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func (s *Service) comparePassword(hashed, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password)) == nil
}

func (s *Service) Register(ctx context.Context, req models.RegisterRequest) (*models.User, *models.Tenant, error) {
	existing, err := s.users.FindUserByEmail(ctx, req.Email)
	switch {
	case err == nil && existing != nil:
		return nil, nil, errs.ErrAlreadyExists
	case errors.Is(err, errs.ErrNotFound):
	case err != nil:
		return nil, nil, err
	}

	hashed, err := s.hashPassword(req.Password)
	if err != nil {
		return nil, nil, err
	}

	user := &models.User{
		Email:        req.Email,
		PasswordHash: hashed,
		Name:         req.Name,
		Role:         enums.RoleUser,
	}
	TenantName, err := CreateTenantName(req.Name)
	if err != nil {
		TenantName = consts.DefaultWorkspace
	}
	tenant, err := s.userWriter.CreateUserWithTenant(ctx, user, TenantName)
	if err != nil {
		return nil, nil, err
	}
	user.PasswordHash = ""
	return user, tenant, nil
}

func (s *Service) Login(ctx context.Context, req models.LoginRequest, clientIP, userAgent string) (*models.LoginResponse, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	user, err := s.users.FindUserByEmail(ctx, email)
	hash := string(dummyHash)
	switch {
	case err == nil:
		hash = user.PasswordHash
	case errors.Is(err, errs.ErrNotFound):
		user = nil
	default:
		return nil, err
	}
	if !s.comparePassword(hash, req.Password) || user == nil {
		return nil, errs.ErrInvalidCredentials
	}

	choices, err := s.membership.GetUserTenantChoices(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	switch len(choices) {
	case 0:
		return nil, errs.ErrNoTenantMembership
	case 1:
		return s.issueSession(ctx, user, choices[0].TenantID, clientIP, userAgent)
	default:
		return s.startTenantSelection(ctx, user, choices)
	}
}

func (s *Service) SelectTenant(ctx context.Context, req models.SelectTenantRequest, clientIP, userAgent string) (*models.LoginResponse, error) {
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil || tenantID == uuid.Nil {
		return nil, errs.ErrInvalidID
	}

	// Atomic single-use consume: replays and concurrent requests fail here.
	raw, err := s.challenges.ConsumeLoginChallenge(ctx, utils.ChallengeKey(req.SelectionToken))
	if err != nil {
		if errors.Is(err, storage.ErrChallengeNotFound) {
			return nil, errs.ErrInvalidSelectionToken
		}
		return nil, err
	}
	var ch loginChallenge
	if err := json.Unmarshal(raw, &ch); err != nil || ch.UserID == uuid.Nil {
		return nil, errs.ErrInvalidSelectionToken
	}

	// Re-check the CURRENT membership; access may have been revoked since login.
	if _, err := s.membership.GetMembership(ctx, tenantID, ch.UserID); err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, errs.ErrTenantAccessDenied
		}
		return nil, err
	}

	user, err := s.users.FindUserByID(ctx, ch.UserID)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, errs.ErrInvalidSelectionToken
		}
		return nil, err
	}

	return s.issueSession(ctx, user, tenantID, clientIP, userAgent)
}

func (s *Service) issueSession(ctx context.Context, user *models.User, tenantID uuid.UUID, clientIP, userAgent string) (*models.LoginResponse, error) {
	pair, err := s.tokens.GenerateTokenPair(ctx, user, tenantID, clientIP, userAgent)
	if err != nil {
		return nil, err
	}
	safe := *user
	safe.PasswordHash = ""
	safe.TenantID = tenantID
	return &models.LoginResponse{
		Status:       "authenticated",
		Token:        pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		TokenType:    pair.TokenType,
		ExpiresIn:    pair.ExpiresIn,
		TenantID:     &tenantID,
		User:         safe,
	}, nil
}

func (s *Service) startTenantSelection(ctx context.Context, user *models.User, choices []models.TenantChoice) (*models.LoginResponse, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate selection token: %w", err)
	}
	token := consts.TokenSelectionPrefix + base64.RawURLEncoding.EncodeToString(buf)

	err := s.challenges.SaveLoginChallenge(ctx, utils.ChallengeKey(token),
		loginChallenge{UserID: user.ID, IssuedAt: time.Now()}, consts.LoginChallengeTTL)
	if err != nil {
		return nil, fmt.Errorf("save login challenge: %w", err)
	}

	safe := *user
	safe.PasswordHash = ""
	return &models.LoginResponse{
		Status:             "tenant_selection_required",
		User:               safe,
		Tenants:            choices,
		SelectionToken:     token,
		SelectionExpiresIn: int64(consts.LoginChallengeTTL.Seconds()),
	}, nil
}

func (s *Service) ParseToken(tokenString string) (*models.UserClaims, error) {
	claims := &models.UserClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errs.ErrInvalidToken
		}
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return nil, errs.ErrInvalidToken
	}
	return claims, nil
}

func (s *Service) GetByID(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	if userID == uuid.Nil {
		return nil, errs.ErrInvalidID
	}
	user, err := s.users.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.PasswordHash = ""
	return user, nil
}

func (s *Service) GetUserTenants(ctx context.Context, userID uuid.UUID) ([]models.Tenant, error) {
	return s.membership.GetUserTenants(ctx, userID)
}

func (s *Service) GetMembership(ctx context.Context, tenantID, userID uuid.UUID) (*models.TenantMember, error) {
	return s.membership.GetMembership(ctx, tenantID, userID)
}

func CreateTenantName(userName string) (string, error) {
	if userName == "" {
		return "", errs.ErrMissingUserName
	}
	return strings.ToUpper(userName[:1]) + userName[1:] + "'s " + strings.ToUpper(consts.DefaultWorkspace[:1]) + consts.DefaultWorkspace[1:], nil
}
