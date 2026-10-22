package consts

import (
	"time"
)

type AIAction string

const MaxDocumentSize = 50 * 1024 * 1024

const (
	SessionTTL         = 30 * time.Minute
	SuggestionCacheTTL = time.Hour
	UnstuckCacheTTL    = 30 * time.Minute
	CostSpendTTL       = 25 * time.Hour
	InvitationTTL      = 24 * time.Hour
)

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

const (
	PDF  = "application/pdf"
	DOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
)

const (
	PdfMIME  = PDF
	DocxMIME = DOCX
)

type DocumentStatus string

const (
	DocumentStatusPending    DocumentStatus = "pending"
	DocumentStatusProcessing DocumentStatus = "processing"
	DocumentStatusActive     DocumentStatus = "active"
	DocumentStatusFailed     DocumentStatus = "failed"
	DocumentStatusArchived   DocumentStatus = "archived"
)

const (
	DefaultDOCXMaxEntries = 5000
	DefaultDOCXMaxXMLSize = 64 << 20
	MaxHeadingRunes       = 200
	MaxTitleRunes         = 200
	HeaderDedupeWindow    = 20
)

const (
	Authorization = "Authorization"
	OpaqueTokenID = "opaque_token_id"
	UserID        = "user_id"
	UserEmail     = "user_email"
	TenantID      = "tenant_id"
	IsService     = "is_service"
	ClientID      = "client_id"
)

const (
	InvitationExchange        = "invitations"
	InvitationQueue           = "invitations.email"
	InvitationRetryQueue      = "invitations.email.retry"
	InvitationDeadQueue       = "invitations.email.dead"
	InvitationRoutingKey      = "email"
	InvitationRetryRoutingKey = "email.retry"
	InvitationDeadRoutingKey  = "email.dead"

	InvitationMaxAttempts = 5
	InvitationRetryDelay  = 30 * time.Second
)
