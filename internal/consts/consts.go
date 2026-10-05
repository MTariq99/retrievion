package consts

import "time"

type AIAction string

const MaxDocumentSize = 50 * 1024 * 1024

const (
	SessionTTL         = 30 * time.Minute
	SuggestionCacheTTL = time.Hour
	UnstuckCacheTTL    = 30 * time.Minute
	CostSpendTTL       = 25 * time.Hour
)

var allowedContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

const (
	MaxChildTokens  = 700
	MinChildTokens  = 50
	OverlapFraction = 0.15
)

type StreamEventType string

const (
	StreamEventToken StreamEventType = "token"
	StreamEventDone  StreamEventType = "done"
	StreamEventError StreamEventType = "error"
)

type MessageRole string

const (
	MessageRoleSystem    MessageRole = "system"
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

const (
	RoutingKey = "ingestion.jobs"
	// Main exchange + queue (live traffic)
	IngestionExchange   = "ingestion.exchange"
	IngestionQueue      = "ingestion.jobs"
	IngestionRoutingKey = "ingestion.jobs"

	// Dead-letter exchange + queue (failed/expired messages)
	DeadLetterExchange   = "ingestion.dlx"
	DeadLetterQueue      = "ingestion.jobs.dlq"
	DeadLetterRoutingKey = "ingestion.jobs.dead"
)

const (
	DefaultRetrievalLimit = 20
	FinalRetrievalTopN    = 10
)
