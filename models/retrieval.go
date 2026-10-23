package models

import (
	"github.com/google/uuid"
)

type RetrievalQuery struct {
	Query          string
	QueryEmbedding []float32
	UserID         uuid.UUID
	TenantID       uuid.UUID
	Limit          int
}

type RetrievalFilters struct {
	DocumentIDs []uuid.UUID
	Sections    []string
}

type Query struct {
	TenantID uuid.UUID
	UserID   uuid.UUID

	Text string

	TopK int
}

type RetrievedChunk struct {
	ChunkID           uuid.UUID
	DocumentID        uuid.UUID
	DocumentVersionID uuid.UUID
	DocumentName      string
	Content           string
	Page              *int
	Section           *string
	Score             float64
	VectorScore       float64
	KeywordScore      float64
}

type Result struct {
	Chunks []RetrievedChunk
}
