package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/consts"
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
	Role      consts.Role
	TokenHash []byte
}

type PendingInvite struct {
	Name  string
	Email string
	Role  consts.Role
	Token string
	Hash  []byte
}

type InvitationEmailMessage struct {
	MessageID     string      `json:"message_id"`
	TenantID      uuid.UUID   `json:"tenant_id"`
	Email         string      `json:"email"`
	Role          consts.Role `json:"role"`
	WorkspaceName string      `json:"workspace_name"`
	Token         string      `json:"token"`
	CreatedAt     time.Time   `json:"created_at"`
}
