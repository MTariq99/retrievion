package errs

import "errors"

var (
	// ErrValidation is the generic validation error.
	ErrValidation = errors.New("validation failed")

	// IDs
	ErrInvalidID = errors.New("invalid id")

	ErrMissingID         = errors.New("id is required")
	ErrMissingUserID     = errors.New("user id is required")
	ErrMissingDocumentID = errors.New("document id is required")

	// User
	ErrMissingName  = errors.New("name is required")
	ErrMissingPhone = errors.New("phone number is required")

	// Authentication
	ErrMissingEmail    = errors.New("email is required")
	ErrInvalidEmail    = errors.New("invalid email")
	ErrMissingPassword = errors.New("password is required")
	ErrInvalidPassword = errors.New("invalid password")

	// Pagination
	ErrInvalidPage     = errors.New("invalid page")
	ErrInvalidPageSize = errors.New("invalid page size")

	// File validation
	ErrInvalidFileExtension = errors.New("invalid file extension")
	ErrInvalidFileSize      = errors.New("invalid file size")

	// Query validation
	ErrMissingQuery = errors.New("query is required")
	ErrInvalidQuery = errors.New("invalid query")

	// Chat validation
	ErrMissingConversationID  = errors.New("conversation id is required")
	ErrMissingMessage         = errors.New("message is required")
	ErrInvalidMessage         = errors.New("invalid message")
	ErrInvalidTitle           = errors.New("conversation title must be provided")
	ErrInvalidInvitation      = errors.New("atleast one invitation is required")
	ErrInvalidTenantID        = errors.New("invalid tenant id")
	ErrTenantIDRequired       = errors.New("tenant ID is required")
	ErrGenerateToken          = errors.New("error generating token")
	ErrTokenNotFound          = errors.New("token not found")
	ErrTokenAlreadyConsumed   = errors.New("token already consumed")
	ErrRefreshReuseDetected   = errors.New("refresh token reuse detected")
	ErrInvalidInvitationToken = errors.New("invalid invitation token")
	ErrDeliveryInProgress     = errors.New("invitation delivery in progress on another worker")
)
