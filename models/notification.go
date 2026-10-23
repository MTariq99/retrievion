package models

import "github.com/mtariq99/retrievion/internal/consts"

type UserInvitationTemplateData struct {
	Name          string
	Email         string
	Title         string
	Message       string
	WorkspaceName string
	Role          consts.Role
	InvitationURL string
	Expiry        string
	PrivacyURL    string
	TermsURL      string
	SupportURL    string
	Year          int
}
