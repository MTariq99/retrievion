package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/consts"
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
