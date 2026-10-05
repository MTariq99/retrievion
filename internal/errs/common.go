package errs

import "errors"

var (
	// ErrNotFound indicates that the requested resource does not exist.
	ErrNotFound = errors.New("resource not found")

	// ErrAlreadyExists indicates that a resource already exists.
	ErrAlreadyExists = errors.New("resource already exists")

	// ErrConflict indicates that the requested operation conflicts
	// with the current state of the resource.
	ErrConflict = errors.New("conflict")

	// ErrUnauthorized indicates that authentication is required
	// or the provided credentials are invalid.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates that the authenticated user does not
	// have permission to perform the requested operation.
	ErrForbidden = errors.New("forbidden")

	// ErrInvalidRequest indicates that the request itself is malformed.
	ErrInvalidRequest = errors.New("invalid request")

	// ErrInternal is the safe error exposed to clients for unexpected
	// internal failures.
	ErrInternal = errors.New("internal error")

	ErrMissingRequiredFields = errors.New("missing required fields")
)
