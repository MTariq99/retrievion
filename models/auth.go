package models

import (
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/enums"
)

type Tenant struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type User struct {
	ID           uuid.UUID      `json:"id" db:"id"`
	TenantID     uuid.UUID      `json:"tenant_id"`
	Email        string         `json:"email" db:"email"`
	PasswordHash string         `json:"-" db:"password_hash"`
	Name         string         `json:"name" db:"name"`
	CreatedAt    time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at" db:"updated_at"`
	DeletedAt    time.Time      `json:"deleted_at"`
	Role         enums.UserRole `json:"role"`
}

type TenantMember struct {
	TenantID   uuid.UUID `json:"tenant_id" db:"tenant_id"`
	TenantName string
	UserID     uuid.UUID   `json:"user_id" db:"user_id"`
	Role       consts.Role `json:"role" db:"role"`
	CreatedAt  time.Time   `json:"created_at" db:"created_at"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

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

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type TenantChoice struct {
	TenantID uuid.UUID   `json:"tenant_id"`
	Name     string      `json:"name"`
	Role     consts.Role `json:"role"`
}

type LoginResponse struct {
	Status             string         `json:"status"` // "authenticated" | "tenant_selection_required"
	Token              string         `json:"token,omitempty"`
	RefreshToken       string         `json:"refresh_token,omitempty"`
	TokenType          string         `json:"token_type,omitempty"`
	ExpiresIn          int64          `json:"expires_in,omitempty"`
	TenantID           *uuid.UUID     `json:"tenant_id,omitempty"`
	User               User           `json:"user"`
	Tenants            []TenantChoice `json:"tenants,omitempty"`
	SelectionToken     string         `json:"selection_token,omitempty"`
	SelectionExpiresIn int64          `json:"selection_expires_in,omitempty"`
}

type SelectTenantRequest struct {
	SelectionToken string `json:"selection_token" binding:"required"`
	TenantID       string `json:"tenant_id" binding:"required"`
}
