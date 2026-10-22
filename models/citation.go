package models

import (
	"time"

	"github.com/google/uuid"
)

// MessageSource maps to the `message_sources` table (migration 007).
// This is the application-controlled mapping from a bare [N] marker in
// generated text back to a real, verifiable chunk — the LLM never
// generates this data itself (Section 73 of the architecture doc:
// "don't let the LLM generate citation URLs itself").
type MessageSource struct {
	ID            uuid.UUID `json:"id" db:"id"`
	MessageID     uuid.UUID `json:"message_id" db:"message_id"`
	ChunkID       uuid.UUID `json:"chunk_id" db:"chunk_id"`
	DocumentID    uuid.UUID `json:"document_id" db:"document_id"`
	CitationIndex int       `json:"citation_index" db:"citation_index"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// Citation is the API-facing, human-readable projection returned in chat
// responses (Section 65 of the architecture doc) — resolved by joining
// MessageSource against documents/document_chunks, not a direct DB row.
type Citation struct {
	Index        int       `json:"citation"`
	DocumentID   uuid.UUID `json:"document_id"`
	DocumentName string    `json:"document_name"`
	Page         *int      `json:"page,omitempty"`
	Section      *string   `json:"section,omitempty"`
}
