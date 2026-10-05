package logger

import (
	"context"
	"log/slog"
	"os"
)

// contextKey avoids collisions with other packages' context keys —
// standard Go practice, not AI-specific, but critical here since EVERY
// log call downstream needs to reliably pull the same request_id.
type contextKey string

const (
	requestIDKey contextKey = "request_id"
	traceIDKey   contextKey = "trace_id"
	tenantIDKey  contextKey = "tenant_id"
)

var base *slog.Logger

func Init() {
	// JSON output — machine-parseable, per Section 3.1's structured
	// logging requirement. NOT fmt.Println anywhere in business code
	// (matches your own pkg/logger's implied purpose).
	base = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// WithRequestID / WithTraceID / WithTenantID: attach correlation IDs to
// a context ONCE (typically in middleware), and every subsequent log
// call anywhere downstream automatically includes them — this is what
// makes "find every log line for this one request" possible without
// manually threading request_id through every function signature.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey, id)
}

func WithTenantID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, tenantIDKey, id)
}

// FromContext builds a *slog.Logger pre-populated with whatever
// correlation IDs are present in ctx — this is the function every
// service/handler in Retrievion should call instead of a bare logger.
func FromContext(ctx context.Context) *slog.Logger {
	l := base
	if v, ok := ctx.Value(requestIDKey).(string); ok && v != "" {
		l = l.With("request_id", v)
	}
	if v, ok := ctx.Value(traceIDKey).(string); ok && v != "" {
		l = l.With("trace_id", v)
	}
	if v, ok := ctx.Value(tenantIDKey).(string); ok && v != "" {
		l = l.With("tenant_id", v)
	}
	return l
}

// SENSITIVE FIELD DISCIPLINE (Section 3.2): callers must pass only
// pre-sanitized values into structured fields. This helper exists so
// there's ONE place documenting what's forbidden, rather than every
// call site needing to remember independently.
//
// NEVER pass into any log field:
//   - raw API keys, JWTs, session tokens
//   - full user passwords or password hashes
//   - unredacted PII fields the user didn't explicitly consent to logging
//   - full raw retrieved document content, if tenant data sensitivity requires redaction
//
// Prefer logging IDs (chunk_id, document_id) and letting a human look
// up content via a separate, access-controlled query when investigating.
func Redact(s string, keepChars int) string {
	if len(s) <= keepChars {
		return "***"
	}
	return s[:keepChars] + "***"
}
