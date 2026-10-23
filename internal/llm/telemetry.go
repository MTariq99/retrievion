package llm

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mtariq99/retrievion/pkg/logger"
	"github.com/mtariq99/retrievion/pkg/metrics"
)

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
	ThoughtsTokens int
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

func Record(ctx context.Context, pool *pgxpool.Pool, t RunTelemetry) {
	log := logger.FromContext(ctx)

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
		log.Error("failed to persist llm run telemetry", "error", err)
	}
}

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

func StartTimer() func() int64 {
	start := time.Now()
	return func() int64 {
		return time.Since(start).Milliseconds()
	}
}
