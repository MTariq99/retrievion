package errs

import "errors"

var (
	// ErrInvalidCredentials is intentionally generic to avoid revealing
	// whether an email exists in the system.
	ErrInvalidCredentials = errors.New("invalid email or password")

	ErrEmailNotVerified = errors.New("email is not verified")
	ErrAccountDisabled  = errors.New("account is disabled")
	ErrAccountLocked    = errors.New("account is locked")

	ErrInvalidToken = errors.New("invalid token")

	ErrPasswordMismatch         = errors.New("passwords do not match")
	ErrCurrentPasswordIncorrect = errors.New("current password is incorrect")

	ErrMissingProvider = errors.New("provider is required")
	ErrMissingOTP      = errors.New("OTP is required")
)
