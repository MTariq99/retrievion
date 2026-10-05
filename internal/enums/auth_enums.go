package enums

type UserRole string

const (
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
)

type KYCStatus string

const (
	KYCStatusPending  KYCStatus = "pending"
	KYCStatusVerified KYCStatus = "verified"
	KYCStatusRejected KYCStatus = "rejected"
)

const (
	DefaultWorkspace = "Personal workspace"
)
