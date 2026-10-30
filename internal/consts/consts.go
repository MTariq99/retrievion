package consts

import "time"

// ============================================================
// AI / LLM / EMBEDDINGS
// ============================================================

const (
	DefaultGeminiEmbeddingModel = "gemini-embedding-001"
	GeminiBaseURL               = "https://generativelanguage.googleapis.com/v1beta"
	GeminiDimension             = 1536
	GeminiMaxBatchSize          = 100
	ServiceBatchCap             = GeminiMaxBatchSize
	FaithfulnessThreshold       = 0.7
	GeminiProvider              = "gemini"
)

// LLM generation retry and timeout policy.
const (
	LLMMaxRetries         = 3
	LLMBaseBackoff        = 500 * time.Millisecond
	LLMMaxBackoff         = 6 * time.Second
	LLMDefaultCallTimeout = 45 * time.Second
	LLMStreamCallTimeout  = 90 * time.Second
)

const (
	GeminiTimeout = 30 * time.Second
	MaxRetries    = 4
	BaseBackoff   = 500 * time.Millisecond
	MaxBackoff    = 8 * time.Second
)

// ============================================================
// AUTHENTICATION / CONTEXT / OTP
// ============================================================
type ReplayVerdict int

const (
	Authorization                      = "Authorization"
	OpaqueTokenID                      = "opaque_token_id"
	UserID                             = "user_id"
	UserEmail                          = "user_email"
	TenantID                           = "tenant_id"
	IsService                          = "is_service"
	ClientID                           = "client_id"
	UserRole                           = "user_role"
	AccessTokenTTL                     = 24 * time.Hour
	RefreshTokenTTL                    = 7 * 24 * time.Hour
	TokenPrefix                        = "tok_"
	LoginChallengeTTL                  = 5 * time.Minute
	LoginChallengePrefix               = "login_challenge:"
	TokenSelectionPrefix               = "sel_"
	RefreshReuseGrace                  = 10 * time.Second
	ReplayRetry          ReplayVerdict = iota // within grace: reject, keep sessions
	ReplayTheft                               // after grace: revoke everything
	DefaultOutboxBatch   = 50
	ReclaimAfter         = 5 * time.Minute // must exceed the longest send (20s timeout)
	StatusTimeout        = 5 * time.Second
)

type DeliveryState int

const (
	DeliveryClaimed DeliveryState = iota
	DeliveryAlreadySent
	DeliveryInProgress
)

type ContextKey string

const (
	RequestIDKey ContextKey = "request_id"
	TraceIDKey   ContextKey = "trace_id"
	TenantIDKey  ContextKey = "tenant_id"
)

const (
	SessionTTL   = 30 * time.Minute
	CostSpendTTL = 25 * time.Hour
)

const (
	OtpVerificationTTL   = 10 * time.Minute
	OtpForgotPasswordTTL = 10 * time.Minute

	PrefixOTPVerification   = "otp:verification"
	PrefixOTPForgotPassword = "otp:forgot_password"
)

// ============================================================
// DOCUMENTS / MIME TYPES / FILE SIGNATURES
// ============================================================

const MaxDocumentSize = 50 * 1024 * 1024

const (
	PDFMIME  = "application/pdf"
	DOCXMIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
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
	PdfMagic         = "%PDF-"
	ZipMagic         = "PK\x03\x04"
	ContentTypesXML  = "[Content_Types].xml"
	DocxMainPartType = "wordprocessingml.document.main+xml"
	MaxContentTypes  = 1 << 20
)

// ============================================================
// DOCUMENT INGESTION / DOCX PARSING
// ============================================================

const (
	DefaultDOCXMaxEntries = 5000
	DefaultDOCXMaxXMLSize = 64 << 20
	MaxHeadingRunes       = 200
	MaxTitleRunes         = 200
	HeaderDedupeWindow    = 20
)

const (
	NsWordTransitional  = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	NsWordStrict        = "http://purl.oclc.org/ooxml/wordprocessingml/main"
	NsMC                = "http://schemas.openxmlformats.org/markup-compatibility/2006"
	DefaultDOCXMainPart = "word/document.xml"
)

type BlockKind uint8

const (
	KindParagraph BlockKind = iota
	KindHeading
	KindListItem
)

type JobStatus string

const (
	JobStatusQueued     JobStatus = "queued"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
	JobStatusDeadLetter JobStatus = "dead_letter"
)

// ============================================================
// CHUNKING
// ============================================================

const (
	MaxChildTokens  = 700
	MinChildTokens  = 50
	OverlapFraction = 0.15
	MaxParentTokens = 2000
)

const SectionSeparator = "\n\n"

// ============================================================
// RETRIEVAL
// ============================================================

const (
	DefaultRetrievalLimit = 20
	FinalRetrievalTopN    = 10
	DefaultCohereEndpoint = "https://api.cohere.com/v2/rerank"
	DefaultCohereModel    = "rerank-v5.0-fast"
	MinTimeout            = 10 * time.Second
	MaxDocuments          = 1000
	MaxErrorBody          = 4 << 10  // 4 KiB
	MaxResponseBody       = 10 << 20 // 10 MiB
	MaxRerankRetries      = 2
)

// ============================================================
// MESSAGE ROLES / STREAMING
// ============================================================

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

// ============================================================
// INGESTION QUEUE / RABBITMQ
// ============================================================

const (
	IngestionExchangeName           = "ingestion.exchange"
	IngestionQueueName              = "ingestion.jobs"
	IngestionRoutingKey             = "ingestion.jobs"
	IngestionDeadLetterExchangeName = "ingestion.dlx"
	IngestionDeadLetterQueueName    = "ingestion.jobs.dlq"
	IngestionDeadLetterRoutingKey   = "ingestion.jobs.dead"
	EventTypeIngestionJobCreated    = "ingestion.job.created"
)

// ============================================================
// INVITATIONS
// ============================================================
type InvitationStatus string

const (
	InvitationTTL = 24 * time.Hour

	InvitationExchange        = "invitations"
	InvitationQueue           = "invitations.email"
	InvitationRetryQueue      = "invitations.email.retry"
	InvitationDeadQueue       = "invitations.email.dead"
	InvitationRoutingKey      = "email"
	InvitationRetryRoutingKey = "email.retry"
	InvitationDeadRoutingKey  = "email.dead"
	InvitationMaxAttempts     = 5
	InvitationRetryDelay      = 30 * time.Second
)

const (
	InvitationStatusPending  InvitationStatus = "pending"
	InvitationStatusAccepted InvitationStatus = "accepted"
	InvitationStatusRevoked  InvitationStatus = "revoked"
)

// ============================================================
// DATABASE / POSTGRESQL
// ============================================================

const (
	DriverName             = "postgres"
	DefaultDatabase        = "postgres"
	DefaultMaxOpenConns    = 50
	DefaultMaxIdleConns    = 10
	ConnectionTimeout      = 10 * time.Second
	DefaultConnMaxLifetime = 5 * time.Minute
)

// ============================================================
// TENANCY / MEMBERSHIP ROLES
// ============================================================

type Role string

const (
	RoleOwner        Role = "owner"
	RoleAdmin        Role = "admin"
	RoleMember       Role = "member"
	RoleViewer       Role = "viewer"
	DefaultWorkspace      = "workspace"
)

// ============================================================
// RUNTIME
// ============================================================

const ShutdownTimeout = 30 * time.Second
