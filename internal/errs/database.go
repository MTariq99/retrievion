package errs

import "errors"

var (
	// ErrRetryable indicates that an operation may succeed if retried.
	//
	// Typical examples:
	//   - PostgreSQL serialization failure
	//   - PostgreSQL deadlock
	ErrRetryable = errors.New("temporary database error")

	// ErrDatabaseUnavailable indicates that the database is currently
	// unavailable or unable to accept requests.
	ErrDatabaseUnavailable = errors.New("database unavailable")

	// ErrInvalidReference indicates a foreign-key/reference violation.
	ErrInvalidReference = errors.New("invalid reference")

	// ErrDatabaseTimeout indicates that a database operation exceeded
	// its allowed execution time.
	ErrDatabaseTimeout = errors.New("database operation timed out")

	// ErrTransactionFailed indicates that a database transaction failed.
	ErrTransactionFailed = errors.New("transaction failed")
)
