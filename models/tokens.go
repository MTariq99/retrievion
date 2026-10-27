package models

import (
	"time"

	"github.com/google/uuid"
)

type AccessToken struct {
	ID        string
	UserID    string
	Hash      string
	Scopes    []string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	FamilyID  uuid.UUID
	TokenHash string
	Used      bool
	IP        *string
	UserAgent *string
	ExpiresAt time.Time
	CreatedAt time.Time
	UsedAt    *time.Time
}

type OpaqueToken struct {
	TokenID   string    `json:"token_id" redis:"token_id"`
	UserID    uuid.UUID `json:"user_id" redis:"user_id"`
	TenantID  uuid.UUID `json:"tenant_id" redis:"tenant_id"`
	UserEmail string    `json:"user_email" redis:"user_email"`
	UserName  string    `json:"user_name" redis:"user_name"`
	UserRole  string    `json:"user_role" redis:"user_role"`
	TokenType string    `json:"token_type" redis:"token_type"` // "access" or "refresh"
	ExpiresAt time.Time `json:"expires_at" redis:"expires_at"`
	IssuedAt  time.Time `json:"issued_at" redis:"issued_at"`
	Scopes    []string  `json:"scopes" redis:"scopes"`
	ClientIP  string    `json:"client_ip" redis:"client_ip"`
	UserAgent string    `json:"user_agent" redis:"user_agent"`
	Revoked   bool      `json:"revoked" redis:"revoked"`
	IsService bool      `json:"is_service" redis:"is_service"`
	ClientID  string    `json:"client_id" redis:"client_id"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

type IntrospectionResponse struct {
	Active    bool      `json:"active"`
	UserID    uuid.UUID `json:"user_id,omitempty"`
	TenantID  uuid.UUID `json:"tenant_id,omitempty"`
	UserEmail string    `json:"user_email,omitempty"`
	UserName  string    `json:"user_name,omitempty"`
	UserRole  string    `json:"user_role,omitempty"`
	TokenType string    `json:"token_type,omitempty"`
	ExpiresAt time.Time `json:"exp,omitempty"`
	IssuedAt  time.Time `json:"iat,omitempty"`
	Scopes    []string  `json:"scopes,omitempty"`
	IsService bool      `json:"is_service,omitempty"`
	ClientID  string    `json:"client_id,omitempty"`
}

type ServiceCredential struct {
	ClientID   string
	SecretHash string
	TenantID   uuid.UUID
}
