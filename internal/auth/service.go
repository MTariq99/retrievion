package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/enums"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
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
}

type Service struct {
	users      userReader
	userWriter userWriter
	membership membershipRepo

	jwtSecret    []byte
	jwtTTL       time.Duration
	opaqueTokens *OpaqueTokenService
}

func NewService(repo *Repository, cfg *models.Config, opaqueTokens *OpaqueTokenService) *Service {
	return &Service{
		users:        repo,
		userWriter:   repo,
		membership:   repo,
		jwtSecret:    []byte(cfg.Auth.JWTSecret),
		jwtTTL:       time.Duration(cfg.Auth.JWTExpirationHours) * time.Hour,
		opaqueTokens: opaqueTokens,
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
	user, err := s.users.FindUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, errs.ErrInvalidCredentials
		}
		return nil, err
	}

	if !s.comparePassword(user.PasswordHash, req.Password) {
		return nil, errs.ErrInvalidCredentials
	}

	tokenPair, err := s.opaqueTokens.GenerateTokenPair(ctx, user, user.TenantID, clientIP, userAgent)
	if err != nil {
		return nil, err
	}

	safeUser := *user
	safeUser.PasswordHash = ""

	return &models.LoginResponse{
		Token: tokenPair.AccessToken, // e.g. "tok_xxxxx..." — this is what goes in Authorization: Bearer
		User:  safeUser,
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
