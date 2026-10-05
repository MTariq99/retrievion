package notifications

import (
	"bytes"
	"fmt"
	"html/template"
	"net/smtp"
	"time"

	"github.com/mtariq99/retrievion/models"
)

type EmailService struct {
	host     string
	port     int
	username string
	password string
	from     string
	auth     smtp.Auth
}

func NewEmailService(cfg *models.Config) *EmailService {
	if cfg.SMTP.Host == "" {
		panic("SMTP_HOST is required but not set")
	}
	if cfg.SMTP.Port == 0 {
		panic("SMTP_PORT is required but not set")
	}
	if cfg.SMTP.Username == "" {
		panic("SMTP_USERNAME is required but not set")
	}
	if cfg.SMTP.Password == "" {
		panic("SMTP_PASSWORD is required but not set")
	}
	if cfg.SMTP.From == "" {
		panic("SMTP_FROM is required but not set")
	}

	auth := smtp.PlainAuth("", cfg.SMTP.Username, cfg.SMTP.Password, cfg.SMTP.Host)
	return &EmailService{
		host:     cfg.SMTP.Host,
		port:     cfg.SMTP.Port,
		username: cfg.SMTP.Username,
		password: cfg.SMTP.Password,
		from:     cfg.SMTP.From,
		auth:     auth,
	}
}

// ----------------------------------------------------------------
// Public methods
// ----------------------------------------------------------------

func (e *EmailService) SendEmailVerificationOTP(to, name, otp string) error {
	body, err := e.renderOTPTemplate(emailVerificationTemplate, OTPTemplateData{
		OTP:     otp,
		Title:   "Verify Your Email Address",
		Message: "To complete your Rentivox registration, use the one-time code below. Do not share this code with anyone.",
		Name:    name,
		Year:    time.Now().Year(),
	})
	if err != nil {
		return fmt.Errorf("failed to render verification template: %w", err)
	}
	return e.send(to, "Verify your Rentivox account", body)
}

func (e *EmailService) SendForgotPasswordOTP(to, name, otp string) error {
	body, err := e.renderOTPTemplate(emailVerificationTemplate, OTPTemplateData{
		OTP:     otp,
		Title:   "Reset Your Password",
		Message: "We received a request to reset your Rentivox password. Use the one-time code below to proceed.",
		Name:    name,
		Year:    time.Now().Year(),
	})
	if err != nil {
		return fmt.Errorf("failed to render forgot password template: %w", err)
	}
	return e.send(to, "Reset your Rentivox password", body)
}

// ----------------------------------------------------------------
// Core send
// ----------------------------------------------------------------

func (e *EmailService) send(to, subject, htmlBody string) error {
	if e == nil {
		return fmt.Errorf("email service is not initialized")
	}
	fmt.Println("e.Host,e.Port>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>", e.host, e.port)
	addr := fmt.Sprintf("%s:%d", e.host, e.port)

	headers := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n",
		e.from, to, subject,
	)

	msg := []byte(headers + htmlBody)

	if err := smtp.SendMail(addr, e.auth, e.username, []string{to}, msg); err != nil {
		return fmt.Errorf("failed to send email to %s: %w", to, err)
	}

	return nil
}

// ----------------------------------------------------------------
// Templates
// ----------------------------------------------------------------

type OTPTemplateData struct {
	OTP     string
	Title   string
	Message string
	Name    string
	Year    int
}

func (e *EmailService) renderOTPTemplate(tmpl string, data OTPTemplateData) (string, error) {
	t, err := template.New("email").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

const emailVerificationTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.Title}}</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }

    body {
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
      background-color: #f0f2f5;
      color: #1a1a1a;
    }

    .wrapper {
      width: 100%;
      padding: 48px 16px;
    }

    .container {
      max-width: 520px;
      margin: 0 auto;
      background: #ffffff;
      border-radius: 12px;
      overflow: hidden;
      box-shadow: 0 2px 8px rgba(0,0,0,0.06);
    }

    /* Header */
    .header {
      background: #0f0f0f;
      padding: 28px 40px;
      display: flex;
      align-items: center;
      gap: 10px;
    }

    .logo-mark {
      width: 32px;
      height: 32px;
      background: #ffffff;
      border-radius: 8px;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      font-size: 16px;
      font-weight: 800;
      color: #0f0f0f;
      letter-spacing: -1px;
    }

    .logo-name {
      font-size: 20px;
      font-weight: 700;
      color: #ffffff;
      letter-spacing: -0.3px;
    }

    /* Body */
    .body {
      padding: 40px 40px 32px;
    }

    .greeting {
      font-size: 15px;
      color: #555;
      margin-bottom: 6px;
    }

    .title {
      font-size: 24px;
      font-weight: 700;
      color: #0f0f0f;
      margin-bottom: 16px;
      letter-spacing: -0.4px;
    }

    .message {
      font-size: 15px;
      line-height: 1.6;
      color: #555;
      margin-bottom: 32px;
    }

    /* OTP Box */
    .otp-label {
      font-size: 11px;
      font-weight: 600;
      letter-spacing: 1.2px;
      text-transform: uppercase;
      color: #999;
      margin-bottom: 10px;
    }

    .otp-box {
      background: #f7f7f7;
      border: 1px solid #e8e8e8;
      border-radius: 10px;
      padding: 24px;
      text-align: center;
      margin-bottom: 24px;
    }

    .otp-code {
      font-size: 40px;
      font-weight: 800;
      letter-spacing: 14px;
      color: #0f0f0f;
      font-variant-numeric: tabular-nums;
    }

    .otp-expiry {
      margin-top: 10px;
      font-size: 13px;
      color: #999;
    }

    .otp-expiry span {
      color: #e05c2a;
      font-weight: 600;
    }

    /* Warning */
    .warning {
      background: #fffbf0;
      border: 1px solid #fde68a;
      border-radius: 8px;
      padding: 14px 16px;
      font-size: 13px;
      color: #92710a;
      line-height: 1.5;
      margin-bottom: 32px;
    }

    .warning strong {
      color: #78590a;
    }

    .divider {
      border: none;
      border-top: 1px solid #f0f0f0;
      margin-bottom: 24px;
    }

    .ignore-note {
      font-size: 13px;
      color: #aaa;
      line-height: 1.5;
    }

    /* Footer */
    .footer {
      background: #fafafa;
      border-top: 1px solid #f0f0f0;
      padding: 24px 40px;
      text-align: center;
    }

    .footer-brand {
      font-size: 13px;
      font-weight: 600;
      color: #0f0f0f;
      margin-bottom: 6px;
    }

    .footer-links {
      font-size: 12px;
      color: #bbb;
      margin-bottom: 8px;
    }

    .footer-links a {
      color: #bbb;
      text-decoration: none;
      margin: 0 8px;
    }

    .footer-links a:hover {
      color: #888;
    }

    .footer-copy {
      font-size: 11px;
      color: #ccc;
    }
  </style>
</head>
<body>
  <div class="wrapper">
    <div class="container">

      <!-- Header -->
      <div class="header">
        <div class="logo-mark">V</div>
        <div class="logo-name">Rentivox</div>
      </div>

      <!-- Body -->
      <div class="body">
        <div class="greeting">Hello, {{.Name}}</div>
        <div class="title">{{.Title}}</div>
        <div class="message">{{.Message}}</div>

        <div class="otp-label">Your one-time code</div>
        <div class="otp-box">
          <div class="otp-code">{{.OTP}}</div>
          <div class="otp-expiry">Expires in <span>10 minutes</span></div>
        </div>

        <div class="warning">
          <strong>Never share this code.</strong> Rentivox will never ask for your OTP via phone, chat, or email.
        </div>

        <hr class="divider">

        <div class="ignore-note">
          If you didn't request this, you can safely ignore this email.
          Someone may have entered your email address by mistake.
        </div>
      </div>

      <!-- Footer -->
      <div class="footer">
        <div class="footer-brand">Rentivox</div>
        <div class="footer-links">
          <a href="#">Privacy Policy</a>
          <a href="#">Terms of Service</a>
          <a href="#">Support</a>
        </div>
        <div class="footer-copy">&copy; {{.Year}} Rentivox. All rights reserved.</div>
      </div>

    </div>
  </div>
</body>
</html>
`
