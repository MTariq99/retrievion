package middleware

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/pkg/logger"
	"github.com/mtariq99/retrievion/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
)

func RequestTracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		ctx, span := tracing.StartSpan(r.Context(), r.Method+" "+r.URL.Path)
		defer span.End()

		traceID := span.SpanContext().TraceID().String()

		ctx = logger.WithRequestID(ctx, requestID)
		ctx = logger.WithTraceID(ctx, traceID)

		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Trace-ID", traceID)

		span.SetAttributes(
			attribute.String("http.request_id", requestID),
		)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
