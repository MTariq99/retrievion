package middleware

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/pkg/logger"
	"github.com/mtariq99/retrievion/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// RequestTracing is the FIRST middleware in the chain (before auth,
// before tenant resolution) — everything downstream needs request_id
// and trace context already in place, including error logging for
// requests that fail auth.
func RequestTracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String() // client didn't supply one — generate it
		}

		ctx, span := tracing.StartSpan(r.Context(), r.Method+" "+r.URL.Path)
		defer span.End()

		traceID := span.SpanContext().TraceID().String()

		ctx = logger.WithRequestID(ctx, requestID)
		ctx = logger.WithTraceID(ctx, traceID)

		// Echo both back so a client (or your own debugging curl) can
		// correlate a specific HTTP response to server-side telemetry
		// without needing log access.
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Trace-ID", traceID)

		span.SetAttributes(
			attribute.String("http.request_id", requestID),
		)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
