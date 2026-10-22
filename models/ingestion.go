package models

import (
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

// JobStatus mirrors the CHECK constraint on ingestion_jobs.status
// (migration 009).
type JobStatus string

const (
	JobStatusQueued     JobStatus = "queued"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
	JobStatusDeadLetter JobStatus = "dead_letter"
)

// IngestionJob maps to the `ingestion_jobs` table (migration 009).
// This is what makes the async ingestion pipeline observable — every
// attempt, failure, and retry is tracked here rather than only logged.
type IngestionJob struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	TenantID          uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	DocumentID        uuid.UUID  `json:"document_id" db:"document_id"`
	DocumentVersionID uuid.UUID  `json:"document_version_id" db:"document_version_id"`
	Status            JobStatus  `json:"status" db:"status"`
	Attempts          int        `json:"attempts" db:"attempts"`
	Error             *string    `json:"error,omitempty" db:"error"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	StartedAt         *time.Time `json:"started_at,omitempty" db:"started_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty" db:"completed_at"`
}

// IsRetryable mirrors the bounded-retry policy from Topic 2/4's
// isRetryable pattern, applied here to ingestion job outcomes rather
// than HTTP status codes.
func (j *IngestionJob) IsRetryable(maxAttempts int) bool {
	return j.Status == JobStatusFailed && j.Attempts < maxAttempts
}

// --- Queue message payload (what actually gets published/consumed —
// deliberately minimal, the worker loads full job state from the DB
// using this ID rather than trusting stale data riding in the queue message) ---

type JobMessage struct {
	JobID             uuid.UUID `json:"job_id"`
	DocumentID        uuid.UUID `json:"document_id"`
	DocumentVersionID uuid.UUID `json:"version_id"`
}

type QueueMessage struct {
	Body        []byte
	DeliveryTag uint64
	Delivery    amqp.Delivery
}

type Job struct {
	JobID             uuid.UUID
	DocumentID        uuid.UUID
	DocumentVersionID uuid.UUID
}

// jobPayload mirrors storage.MarshalIngestionJob's wire format.
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
