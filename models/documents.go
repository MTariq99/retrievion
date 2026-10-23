package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/pgvector/pgvector-go"
)

type Document struct {
	ID        uuid.UUID             `json:"id" db:"id"`
	TenantID  uuid.UUID             `json:"tenant_id" db:"tenant_id"`
	Name      string                `json:"name" db:"name"`
	MimeType  string                `json:"mime_type" db:"mime_type"`
	Status    consts.DocumentStatus `json:"status" db:"status"`
	CreatedAt time.Time             `json:"created_at" db:"created_at"`
	UpdatedAt time.Time             `json:"updated_at" db:"updated_at"`
	Versions  []*DocumentVersion    `json:"versions"`
}
type DocumentVersion struct {
	ID          uuid.UUID             `json:"id" db:"id"`
	DocumentID  uuid.UUID             `json:"document_id" db:"document_id"`
	Version     int                   `json:"version" db:"version"`
	ContentHash string                `json:"content_hash" db:"content_hash"`
	StorageKey  string                `json:"-" db:"storage_key"`
	StorageURL  string                `json:"url" db:"-"`
	Status      consts.DocumentStatus `json:"status" db:"status"`
	CreatedAt   time.Time             `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at" db:"updated_at"`
}

type ChunkMetadata map[string]interface{}

func (m ChunkMetadata) Value() ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

type DocumentChunk struct {
	ID                uuid.UUID        `json:"id" db:"id"`
	TenantID          uuid.UUID        `json:"tenant_id" db:"tenant_id"`
	DocumentID        uuid.UUID        `json:"document_id" db:"document_id"`
	DocumentVersionID uuid.UUID        `json:"document_version_id" db:"document_version_id"`
	ParentChunkID     *uuid.UUID       `json:"parent_chunk_id,omitempty" db:"parent_chunk_id"`
	ChunkIndex        int              `json:"chunk_index" db:"chunk_index"`
	Content           string           `json:"content" db:"content"`
	ContentHash       string           `json:"content_hash" db:"content_hash"`
	Embedding         *pgvector.Vector `json:"-" db:"embedding"`
	Page              *int             `json:"page,omitempty" db:"page"`
	Section           *string          `json:"section,omitempty" db:"section"`
	Metadata          ChunkMetadata    `json:"metadata,omitempty" db:"metadata"`
	TokenCount        int              `json:"token_count" db:"token_count"`
	CreatedAt         time.Time        `json:"created_at" db:"created_at"`
}

func (c *DocumentChunk) IsChild() bool {
	return c.ParentChunkID != nil
}

type UploadRequest struct {
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
}

type DocumentStatusResponse struct {
	DocumentID uuid.UUID             `json:"document_id"`
	Status     consts.DocumentStatus `json:"status"`
	Error      *string               `json:"error,omitempty"`
}

type UploadResult struct {
	DocumentID uuid.UUID
	VersionID  uuid.UUID
	JobID      uuid.UUID
	Status     consts.DocumentStatus
}

type PDFOptions struct {
	MaxInputBytes int64
}
