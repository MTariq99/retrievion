package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Every metric here maps directly to a question from the lesson's
// Section 18 ("senior-level engineering questions I should learn to
// ask") — each metric exists to answer ONE specific question, not
// because "more metrics = better observability."

var (
	LLMRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llm_requests_total",
			Help: "Total LLM API calls, labeled by model and outcome.",
		},
		[]string{"model", "provider", "status"}, // status: "success" | "error"
	)

	LLMErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llm_errors_total",
			Help: "LLM call failures, labeled by error category.",
		},
		[]string{"model", "error_type"}, // "timeout" | "rate_limit" | "invalid_request" | ...
	)

	TokensInputTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llm_tokens_input_total",
			Help: "Cumulative input tokens consumed, labeled by tenant/model.",
		},
		[]string{"tenant_id", "model"},
	)

	TokensOutputTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llm_tokens_output_total",
			Help: "Cumulative output tokens generated, labeled by tenant/model.",
		},
		[]string{"tenant_id", "model"},
	)

	CostTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llm_cost_usd_total",
			Help: "Cumulative estimated cost in USD, labeled by tenant/model — answers 'which tenant is expensive.'",
		},
		[]string{"tenant_id", "model"},
	)

	LLMLatencySeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "llm_latency_seconds",
			Help:    "End-to-end LLM call latency. Use histogram, not average — p95/p99 matter more than mean (Section 8).",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 20},
		},
		[]string{"model"},
	)

	TimeToFirstTokenSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "llm_ttft_seconds",
			Help:    "Time to first streamed token — the user-perceived latency signal, distinct from total latency.",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		},
		[]string{"model"},
	)

	RetrievalLatencySeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "retrieval_latency_seconds",
			Help:    "Vector/keyword search latency, separate from LLM latency — isolates WHICH stage is slow.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1},
		},
		[]string{"strategy"}, // "vector" | "keyword" | "hybrid"
	)

	RetrievalHitRate = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "retrieval_hit_rate",
			Help: "Fraction of queries returning at least one result above threshold — a falling value signals ingestion or threshold problems (Topic 9's failure scenarios).",
		},
	)

	EvaluationPassRate = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "evaluation_pass_rate",
			Help: "Latest eval run's pass rate, labeled by dataset — the regression-detection signal for CI/CD gating.",
		},
		[]string{"dataset_name"},
	)

	GroundednessScore = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "groundedness_score",
			Help: "Average faithfulness score of the latest evaluation run.",
		},
		[]string{"dataset_name"},
	)

	ToolFailureTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agent_tool_failure_total",
			Help: "Tool call failures, labeled by tool name — identifies which tool is the weak point in an agent (Topic 11/12).",
		},
		[]string{"tool_name"},
	)

	AgentRunsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agent_runs_total",
			Help: "Agent workflow completions, labeled by outcome.",
		},
		[]string{"outcome"}, // "final_answer" | "max_iterations" | "error"
	)
)
