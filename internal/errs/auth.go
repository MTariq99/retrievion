package errs

import "errors"

var (
	ErrInvalidCredentials       = errors.New("invalid email or password")
	ErrEmailNotVerified         = errors.New("email is not verified")
	ErrAccountDisabled          = errors.New("account is disabled")
	ErrAccountLocked            = errors.New("account is locked")
	ErrInvalidToken             = errors.New("invalid token")
	ErrPasswordMismatch         = errors.New("passwords do not match")
	ErrCurrentPasswordIncorrect = errors.New("current password is incorrect")
	ErrMissingProvider          = errors.New("provider is required")
	ErrMissingOTP               = errors.New("OTP is required")
	ErrMissingUserName          = errors.New("user name is required")
	ErrNoTenantMembership       = errors.New("account has no workspace membership")
	ErrInvalidSelectionToken    = errors.New("invalid or expired selection token")
	ErrTenantAccessDenied       = errors.New("no access to the selected workspace")
	ErrChallengeNotFound        = errors.New("challenge not found")
)
