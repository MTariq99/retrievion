package models

type UserInvitationTemplateData struct {
	Name          string
	Email         string
	Title         string
	Message       string
	WorkspaceName string
	Role          string
	InvitationURL string
	Expiry        string
	PrivacyURL    string
	TermsURL      string
	SupportURL    string
	Year          int
}
