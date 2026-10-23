package models

import (
	"time"

	"github.com/google/uuid"
)

type MessageSource struct {
	ID            uuid.UUID `json:"id" db:"id"`
	MessageID     uuid.UUID `json:"message_id" db:"message_id"`
	ChunkID       uuid.UUID `json:"chunk_id" db:"chunk_id"`
	DocumentID    uuid.UUID `json:"document_id" db:"document_id"`
	CitationIndex int       `json:"citation_index" db:"citation_index"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

type Citation struct {
	Index        int       `json:"citation"`
	DocumentID   uuid.UUID `json:"document_id"`
	DocumentName string    `json:"document_name"`
	Page         *int      `json:"page,omitempty"`
	Section      *string   `json:"section,omitempty"`
}
