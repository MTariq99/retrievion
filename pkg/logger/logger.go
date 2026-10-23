package logger

import (
	"context"
	"log/slog"
	"os"

	"github.com/mtariq99/retrievion/internal/consts"
)

var base *slog.Logger

func Init() {
	base = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, consts.RequestIDKey, id)
}

func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, consts.TraceIDKey, id)
}

func WithTenantID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, consts.TenantIDKey, id)
}

func FromContext(ctx context.Context) *slog.Logger {
	l := base
	if v, ok := ctx.Value(consts.RequestIDKey).(string); ok && v != "" {
		l = l.With("request_id", v)
	}
	if v, ok := ctx.Value(consts.TraceIDKey).(string); ok && v != "" {
		l = l.With("trace_id", v)
	}
	if v, ok := ctx.Value(consts.TenantIDKey).(string); ok && v != "" {
		l = l.With("tenant_id", v)
	}
	return l
}

func Redact(s string, keepChars int) string {
	if len(s) <= keepChars {
		return "***"
	}
	return s[:keepChars] + "***"
}
