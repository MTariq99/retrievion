package errs

import "errors"

var (
	ErrNotFound      = errors.New("resource not found")
	ErrAlreadyExists = errors.New("resource already exists")
	ErrConflict      = errors.New("conflict")
	ErrUnauthorized  = errors.New("unauthorized")

	ErrForbidden = errors.New("forbidden")

	ErrInvalidRequest = errors.New("invalid request")
	ErrInternal       = errors.New("internal error")

	ErrMissingRequiredFields = errors.New("missing required fields")
	ErrIDRequired            = errors.New("id is required")
)
