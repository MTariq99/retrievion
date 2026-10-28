package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/enums"
	"github.com/mtariq99/retrievion/models"
)

type OpaqueTokenStore interface {
	SaveToken(ctx context.Context, token *models.OpaqueToken) error
	GetToken(ctx context.Context, tokenID string) (*models.OpaqueToken, error)
	RevokeToken(ctx context.Context, tokenID string) error
	RevokeAllUserTokens(ctx context.Context, userID uuid.UUID) error
}
type TenantMembershipReader interface {
	GetMembership(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID) (*models.TenantMember, error)
}

type OpaqueTokenService struct {
	redis      OpaqueTokenStore
	config     *TokenConfig
	membership TenantMembershipReader
}

type TokenConfig struct {
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	TokenPrefix     string
}

func NewTokenConfig() *TokenConfig {
	return &TokenConfig{
		AccessTokenTTL:  consts.AccessTokenTTL,
		RefreshTokenTTL: consts.RefreshTokenTTL,
		TokenPrefix:     consts.TokenPrefix,
	}
}

func NewOpaqueTokenService(redis OpaqueTokenStore, config *TokenConfig, membership TenantMembershipReader) *OpaqueTokenService {
	if config == nil {
		config = NewTokenConfig()
	}
	return &OpaqueTokenService{
		redis:      redis,
		config:     config,
		membership: membership,
	}
}

func (s *OpaqueTokenService) GenerateTokenPair(ctx context.Context, user *models.User, tenantID uuid.UUID, clientIP, userAgent string) (*models.TokenPair, error) {
	accessToken, err := s.generateOpaqueToken(ctx, user, tenantID, "access", s.config.AccessTokenTTL, clientIP, userAgent)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := s.generateOpaqueToken(ctx, user, tenantID, "refresh", s.config.RefreshTokenTTL, clientIP, userAgent)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &models.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.config.AccessTokenTTL.Seconds()),
	}, nil
}

func (s *OpaqueTokenService) generateOpaqueToken(ctx context.Context, user *models.User, tenantID uuid.UUID, tokenType string, ttl time.Duration, clientIP, userAgent string) (string, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	tokenID := base64.URLEncoding.EncodeToString(randomBytes)
	prefix := "tok_"
	if tokenType == "refresh" {
		prefix = "rtok_"
	}
	fullToken := prefix + tokenID

	userEmail := ""
	if user.Email != "" {
		userEmail = user.Email
	}

	now := time.Now()
	opaqueToken := &models.OpaqueToken{
		TokenID:   fullToken,
		UserID:    user.ID,
		UserEmail: userEmail,
		UserName:  user.Name,
		UserRole:  getUserRole(user),
		TokenType: tokenType,
		ExpiresAt: now.Add(ttl),
		IssuedAt:  now,
		Scopes:    []string{"read", "write"},
		ClientIP:  clientIP,
		UserAgent: userAgent,
		Revoked:   false,
		TenantID:  tenantID,
	}

	if err := s.redis.SaveToken(ctx, opaqueToken); err != nil {
		return "", fmt.Errorf("failed to save token: %w", err)
	}
	return fullToken, nil
}

func (s *OpaqueTokenService) IntrospectToken(ctx context.Context, tokenID string) (*models.IntrospectionResponse, error) {
	token, err := s.redis.GetToken(ctx, tokenID)
	if err != nil {
		return &models.IntrospectionResponse{Active: false}, err
	}
	if token.Revoked || time.Now().After(token.ExpiresAt) {
		return &models.IntrospectionResponse{Active: false}, err
	}
	return &models.IntrospectionResponse{
		Active:    true,
		UserID:    token.UserID,
		UserEmail: token.UserEmail,
		UserName:  token.UserName,
		TokenType: token.TokenType,
		ExpiresAt: token.ExpiresAt,
		IssuedAt:  token.IssuedAt,
		Scopes:    token.Scopes,
		TenantID:  token.TenantID,
		UserRole:  token.UserRole,
		IsService: token.IsService,
		ClientID:  token.ClientID,
	}, nil
}

func (s *OpaqueTokenService) RefreshAccessToken(ctx context.Context, refreshTokenID, clientIP, userAgent string) (*models.TokenPair, error) {
	introspection, err := s.IntrospectToken(ctx, refreshTokenID)
	if err != nil || !introspection.Active {
		return nil, fmt.Errorf("invalid or expired refresh token")
	}

	refreshToken, err := s.redis.GetToken(ctx, refreshTokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to get refresh token: %w", err)
	}
	if refreshToken.TokenType != "refresh" {
		return nil, fmt.Errorf("token is not a refresh token")
	}
	if refreshToken.IsService {
		return nil, fmt.Errorf("service tokens cannot be refreshed through user refresh flow")
	}
	if err = s.ValidateUserTenantMembership(ctx, refreshToken.TenantID, refreshToken.UserID); err != nil {
		return nil, fmt.Errorf("refresh token tenant membership is invalid: %w", err)
	}

	user := &models.User{
		ID:    refreshToken.UserID,
		Name:  refreshToken.UserName,
		Email: refreshToken.UserEmail,
		Role:  enums.UserRole(refreshToken.UserRole),
	}

	accessToken, err := s.generateOpaqueToken(ctx, user, refreshToken.TenantID, "access", s.config.AccessTokenTTL, clientIP, userAgent)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new access token: %w", err)
	}

	return &models.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenID,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.config.AccessTokenTTL.Seconds()),
	}, nil
}

func (s *OpaqueTokenService) RevokeToken(ctx context.Context, tokenID string) error {
	return s.redis.RevokeToken(ctx, tokenID)
}

func (s *OpaqueTokenService) RevokeAllUserTokens(ctx context.Context, userID uuid.UUID) error {
	return s.redis.RevokeAllUserTokens(ctx, userID)
}

func getUserRole(user *models.User) string {
	return string(user.Role)
}

func (s *OpaqueTokenService) GenerateServiceToken(ctx context.Context, clientID string, tenantID uuid.UUID) (string, time.Time, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to generate random token: %w", err)
	}
	fullToken := "stok_" + base64.URLEncoding.EncodeToString(randomBytes)

	now := time.Now()
	expiresAt := now.Add(30 * time.Minute)

	opaqueToken := &models.OpaqueToken{
		TokenID:   fullToken,
		TokenType: "service",
		IsService: true,
		ClientID:  clientID,
		ExpiresAt: expiresAt,
		IssuedAt:  now,
		Scopes:    []string{"rag:retrieve"},
		Revoked:   false,
		TenantID:  tenantID,
	}

	if err := s.redis.SaveToken(ctx, opaqueToken); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to save service token: %w", err)
	}
	return fullToken, expiresAt, nil
}

func (s *OpaqueTokenService) ValidateUserTenantMembership(ctx context.Context, tenantID, userID uuid.UUID) error {
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return fmt.Errorf("tenant and user IDs are required")
	}
	if s.membership == nil {
		return fmt.Errorf("tenant membership reader is not configured")
	}
	membership, err := s.membership.GetMembership(ctx, tenantID, userID)
	if err != nil {
		return fmt.Errorf("get tenant membership: %w", err)
	}
	if membership == nil || membership.TenantID != tenantID || membership.UserID != userID {
		return fmt.Errorf("tenant membership does not match token")
	}
	return nil
}
