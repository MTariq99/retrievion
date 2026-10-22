package models

import (
	"github.com/google/uuid"
)

// RetrievalQuery is what gets passed into retrieval/service.go's
// Retrieve() function.
type RetrievalQuery struct {
	Query          string
	QueryEmbedding []float32
	UserID         uuid.UUID
	TenantID       uuid.UUID
	Limit          int
}

// RetrievalFilters captures the metadata filtering dimensions called out
// in Topic 9 Section 3.9 — tenant is mandatory and handled separately
// (never optional), everything here is additional narrowing.
type RetrievalFilters struct {
	DocumentIDs []uuid.UUID // restrict to specific documents, if set
	Sections    []string
	// Extend here for date ranges, categories, ACL groups, etc. as
	// your product actually needs them — don't add filters speculatively.
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
