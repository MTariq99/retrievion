package jwtclaims

import (
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/enums"
)

type UserClaims struct {
	UserId            uuid.UUID      `json:"user_id"`
	TenantID          uuid.UUID      `json:"tenant_id"`
	UserName          string         `json:"user_name"`
	UserEmail         string         `json:"user_email"`
	UserRole          enums.UserRole `json:"user_role"`
	Scopes            []string       `json:"scopes"`
	UserOpaqueTokenId string         `json:"user_opaque_token_id"`
	IsService         bool           `json:"is_service"`
	ClientID          string         `json:"client_id"`

	jwt.RegisteredClaims
}
