package llm

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mtariq99/retrievion/pkg/logger"
	"github.com/mtariq99/retrievion/pkg/metrics"
)

// RunTelemetry is the Go-side mirror of the llm_runs table
// (migrations/010_observability.up.sql). This is the concrete
// implementation of Section 3.2's JSON example — every field there
// has a home here.
type RunTelemetry struct {
	RequestID      uuid.UUID
	TraceID        string
	TenantID       *uuid.UUID
	UserID         *uuid.UUID
	ConversationID *uuid.UUID

	PromptTemplateID string
	PromptVersion    string

	Model           string
	Provider        string
	Temperature     float64
	MaxOutputTokens int

	InputTokens    int
	OutputTokens   int
	ThoughtsTokens int // your Topic 1 Gemini lesson, tracked permanently
	TotalTokens    int

	LatencyMs          int64
	TimeToFirstTokenMs *int64
	RetrievalLatencyMs *int64
	EmbeddingLatencyMs *int64
	RerankingLatencyMs *int64

	RetrievedChunkIDs []uuid.UUID
	RetrievalScores   []float64

	FinishReason string
	RetryCount   int
	CacheHit     bool

	EstimatedCostUSD float64

	Error string
}

// Record persists telemetry AND emits metrics in one call — called at
// the end of EVERY LLM invocation in the codebase (wrap your existing
// client.go / service.go / streaming.go call sites with this).
//
// Deliberately non-blocking-adjacent design note: in production, this
// write should go through an async queue (Section 8's "telemetry must
// not become a reliability dependency" principle) rather than a
// synchronous INSERT on the request's hot path. This version does a
// direct insert for clarity — see the comment at the bottom for the
// async upgrade path.
func Record(ctx context.Context, pool *pgxpool.Pool, t RunTelemetry) {
	log := logger.FromContext(ctx)

	// --- Metrics first (in-memory, cheap, never fails) ---
	status := "success"
	if t.Error != "" {
		status = "error"
		metrics.LLMErrorsTotal.WithLabelValues(t.Model, classifyError(t.Error)).Inc()
	}
	metrics.LLMRequestsTotal.WithLabelValues(t.Model, t.Provider, status).Inc()

	tenantLabel := "unknown"
	if t.TenantID != nil {
		tenantLabel = t.TenantID.String()
	}
	metrics.TokensInputTotal.WithLabelValues(tenantLabel, t.Model).Add(float64(t.InputTokens))
	metrics.TokensOutputTotal.WithLabelValues(tenantLabel, t.Model).Add(float64(t.OutputTokens))
	metrics.CostTotal.WithLabelValues(tenantLabel, t.Model).Add(t.EstimatedCostUSD)
	metrics.LLMLatencySeconds.WithLabelValues(t.Model).Observe(float64(t.LatencyMs) / 1000.0)

	if t.TimeToFirstTokenMs != nil {
		metrics.TimeToFirstTokenSeconds.WithLabelValues(t.Model).Observe(float64(*t.TimeToFirstTokenMs) / 1000.0)
	}

	// --- Structured log line — human-searchable, correlated via request_id ---
	log.Info(
		"llm run completed",
		"model", t.Model,
		"provider", t.Provider,
		"input_tokens", t.InputTokens,
		"output_tokens", t.OutputTokens,
		"thoughts_tokens", t.ThoughtsTokens,
		"latency_ms", t.LatencyMs,
		"finish_reason", t.FinishReason,
		"estimated_cost_usd", t.EstimatedCostUSD,
		"error", t.Error,
	)

	// --- Durable storage — the queryable record for later debugging ---
	_, err := pool.Exec(
		ctx, `
		INSERT INTO llm_runs (
			request_id, trace_id, tenant_id, user_id, conversation_id,
			prompt_template_id, prompt_version,
			model, model_provider, temperature, max_output_tokens,
			input_tokens, output_tokens, thoughts_tokens, total_tokens,
			latency_ms, time_to_first_token_ms, retrieval_latency_ms,
			embedding_latency_ms, reranking_latency_ms,
			retrieved_chunk_ids, retrieval_scores,
			finish_reason, retry_count, cache_hit,
			estimated_cost_usd, error
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7,
			$8, $9, $10, $11,
			$12, $13, $14, $15,
			$16, $17, $18,
			$19, $20,
			$21, $22,
			$23, $24, $25,
			$26, $27
		)
	`, t.RequestID, t.TraceID, t.TenantID, t.UserID, t.ConversationID,
		t.PromptTemplateID, t.PromptVersion,
		t.Model, t.Provider, t.Temperature, t.MaxOutputTokens,
		t.InputTokens, t.OutputTokens, t.ThoughtsTokens, t.TotalTokens,
		t.LatencyMs, t.TimeToFirstTokenMs, t.RetrievalLatencyMs,
		t.EmbeddingLatencyMs, t.RerankingLatencyMs,
		t.RetrievedChunkIDs, t.RetrievalScores,
		t.FinishReason, t.RetryCount, t.CacheHit,
		t.EstimatedCostUSD, t.Error,
	)
	if err != nil {
		// Deliberate: a telemetry WRITE failure must never fail the
		// user's actual request (Section 8's core principle). Log it
		// loudly and move on — never propagate this error upward.
		log.Error("failed to persist llm run telemetry", "error", err)
	}
}

// classifyError maps a raw error string into a small set of stable
// label values — Prometheus label cardinality must stay bounded
// (Section 8/9's "high-cardinality metrics" failure scenario), so this
// NEVER uses the raw error message itself as a label.
func classifyError(errMsg string) string {
	switch {
	case contains(errMsg, "timeout"):
		return "timeout"
	case contains(errMsg, "429"), contains(errMsg, "rate limit"):
		return "rate_limit"
	case contains(errMsg, "context"), contains(errMsg, "token"):
		return "context_length"
	default:
		return "other"
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()
}

// StartTimer is a tiny helper for the common "measure elapsed ms"
// pattern used to populate LatencyMs / RetrievalLatencyMs / etc.
func StartTimer() func() int64 {
	start := time.Now()
	return func() int64 {
		return time.Since(start).Milliseconds()
	}
}

// NOTE on the async upgrade path referenced above: once request volume
// justifies it, replace the synchronous pool.Exec call with:
//   1. Marshal RunTelemetry to JSON
//   2. Publish to your existing storage/queue.go (RabbitMQ, per your
//      architecture doc)
//   3. A separate worker (parallel to your existing ingestion worker)
//      consumes the queue and performs the INSERT
// This decouples "was the telemetry write slow/failed" from "did the
// user's request succeed" entirely — exactly Section 8's principle,
// taken to its production conclusion.
