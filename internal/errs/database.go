package errs

import "errors"

var (
	ErrRetryable                = errors.New("temporary database error")
	ErrDatabaseUnavailable      = errors.New("database unavailable")
	ErrInvalidReference         = errors.New("invalid reference")
	ErrDatabaseTimeout          = errors.New("database operation timed out")
	ErrTransactionFailed        = errors.New("transaction failed")
	ErrInvitationAlreadyPending = errors.New("an invitation is already pending for this email")
)
