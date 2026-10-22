package models

import (
	"time"

	"github.com/google/uuid"
)

// MessageRole mirrors the CHECK constraint on messages.role (migration 007).
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

// Conversation maps to the `conversations` table (migration 006).
type Conversation struct {
	ID        uuid.UUID `json:"id" db:"id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	Title     *string   `json:"title,omitempty" db:"title"`
	Summary   *string   `json:"summary,omitempty" db:"summary"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
type CreateConversationReq struct {
	Title   *string `json:"title,omitempty"`
	Summary *string `json:"summary,omitempty"`
}

// Message maps to the `messages` table (migration 007).
type Message struct {
	ID             uuid.UUID   `json:"id" db:"id"`
	ConversationID uuid.UUID   `json:"conversation_id" db:"conversation_id"`
	Role           MessageRole `json:"role" db:"role"`
	Content        string      `json:"content" db:"content"`
	TokenCount     int         `json:"token_count" db:"token_count"`
	CreatedAt      time.Time   `json:"created_at" db:"created_at"`
}

// --- Request/response DTOs ---

// ConversationSummary is a lightweight projection for list views
// (GET /conversations) — deliberately excludes full message history.
type ConversationSummary struct {
	ID        uuid.UUID `json:"id"`
	Title     *string   `json:"title,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}
