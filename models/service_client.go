package models

import (
	"time"

	"github.com/google/uuid"
)

type ServiceClient struct {
	ID         uuid.UUID
	ClientID   string
	SecretHash string
	TenantID   uuid.UUID
	Name       string
	Scopes     []string
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

type ServiceTokenRequest struct {
	ClientID     string `json:"client_id" binding:"required"`
	ClientSecret string `json:"client_secret" binding:"required"`
}

type ServiceTokenResponse struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}
