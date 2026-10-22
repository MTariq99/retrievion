package models

import (
	"time"

	"github.com/google/uuid"
)

type InviteUsers struct {
	Users []InviteUser `json:"users"`
}

type InviteUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type NewInvitation struct {
	Name      string
	Email     string
	Role      string
	TokenHash []byte
}

type PendingInvite struct {
	Name  string
	Email string
	Role  string
	Token string
	Hash  []byte
}

type InvitationEmailMessage struct {
	MessageID     string    `json:"message_id"`
	TenantID      uuid.UUID `json:"tenant_id"`
	Email         string    `json:"email"`
	Role          string    `json:"role"`
	WorkspaceName string    `json:"workspace_name"`
	Token         string    `json:"token"` // raw token: never log this struct
	CreatedAt     time.Time `json:"created_at"`
}
