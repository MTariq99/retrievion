package errs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"

	"github.com/jackc/pgconn"
)

func RepositoryError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", ErrDatabaseTimeout, err)
	}

	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}

	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) {
		switch pgErr.Code {

		// unique_violation
		case "23505":
			return ErrAlreadyExists

		// foreign_key_violation
		case "23503":
			return ErrInvalidReference

		// check_violation
		case "23514":
			return ErrValidation

		// not_null_violation
		case "23502":
			return ErrValidation

		// serialization_failure
		case "40001":
			return ErrRetryable

		// deadlock_detected
		case "40P01":
			return ErrRetryable

		// too_many_connections
		case "53300":
			return ErrDatabaseUnavailable

		// cannot_connect_now
		case "57P03":
			return ErrDatabaseUnavailable

		// query_canceled
		case "57014":
			return ErrDatabaseTimeout

		default:
			return fmt.Errorf("postgres error [%s]: %w", pgErr.Code, err)
		}
	}

	// Network-level database failures.
	var netErr net.Error

	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ErrDatabaseTimeout
		}

		return fmt.Errorf("%w: %v", ErrDatabaseUnavailable, err)
	}

	// Preserve the original error for logging/debugging while allowing
	// errors.Is/errors.As to continue working.
	return fmt.Errorf("repository operation failed: %w", err)
}
