package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/consts"
)

type IngestionJob struct {
	ID                uuid.UUID        `json:"id" db:"id"`
	TenantID          uuid.UUID        `json:"tenant_id" db:"tenant_id"`
	DocumentID        uuid.UUID        `json:"document_id" db:"document_id"`
	DocumentVersionID uuid.UUID        `json:"document_version_id" db:"document_version_id"`
	Status            consts.JobStatus `json:"status" db:"status"`
	Attempts          int              `json:"attempts" db:"attempts"`
	Error             *string          `json:"error,omitempty" db:"error"`
	CreatedAt         time.Time        `json:"created_at" db:"created_at"`
	StartedAt         *time.Time       `json:"started_at,omitempty" db:"started_at"`
	CompletedAt       *time.Time       `json:"completed_at,omitempty" db:"completed_at"`
}

func (j *IngestionJob) IsRetryable(maxAttempts int) bool {
	return j.Status == consts.JobStatusFailed && j.Attempts < maxAttempts
}

type JobMessage struct {
	JobID             uuid.UUID `json:"job_id"`
	DocumentID        uuid.UUID `json:"document_id"`
	DocumentVersionID uuid.UUID `json:"version_id"`
}

type Job struct {
	JobID             uuid.UUID
	DocumentID        uuid.UUID
	DocumentVersionID uuid.UUID
	Attempt           int
}
type JobPayload struct {
	JobID             string `json:"job_id"`
	DocumentID        string `json:"document_id"`
	DocumentVersionID string `json:"version_id"`
}

type Page struct {
	Number  int
	Content string
}

type Chunk struct {
	ID                uuid.UUID
	ParentChunkID     *uuid.UUID
	DocumentVersionID uuid.UUID
	Content           string
	ContentHash       string
	Page              int
	Section           string
	ChunkIndex        int
	TokenCount        int
	IsParent          bool
}
