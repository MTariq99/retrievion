package models

import (
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/enums"
)

// Role is the set of valid values for tenant_members.role
// (enforced at the DB level via CHECK constraint, migration 003).
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
	RoleViewer Role = "viewer"
)

// Tenant maps to the `tenants` table (migration 002).
type Tenant struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// User maps to the `users` table (migration 003).
// PasswordHash is deliberately excluded from JSON serialization —
// it should never leave the auth service in an API response.
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

// TenantMember maps to the `tenant_members` join table (migration 003).
// This is what middleware/tenant.go checks to confirm "user belongs to
// tenant" before allowing any request through.
type TenantMember struct {
	TenantID   uuid.UUID `json:"tenant_id" db:"tenant_id"`
	TenantName string
	UserID     uuid.UUID `json:"user_id" db:"user_id"`
	Role       Role      `json:"role" db:"role"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// --- Request/response DTOs (not DB rows — these are what handler.go
// actually parses from/returns to HTTP, kept separate from persistence
// models on purpose so API shape can evolve independently of schema shape) ---

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
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
