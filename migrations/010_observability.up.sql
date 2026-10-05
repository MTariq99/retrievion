-- 010_observability.up.sql
CREATE TABLE llm_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id UUID NOT NULL,
    trace_id TEXT,
    tenant_id UUID REFERENCES tenants (id),
    user_id UUID REFERENCES users (id),
    conversation_id UUID REFERENCES conversations (id),
    prompt_template_id TEXT,
    prompt_version TEXT,
    model TEXT NOT NULL,
    model_provider TEXT NOT NULL,
    temperature DOUBLE PRECISION,
    max_output_tokens INT,
    input_tokens INT NOT NULL DEFAULT 0,
    output_tokens INT NOT NULL DEFAULT 0,
    thoughts_tokens INT NOT NULL DEFAULT 0,
    total_tokens INT NOT NULL DEFAULT 0,
    latency_ms INT,
    time_to_first_token_ms INT,
    retrieval_latency_ms INT,
    embedding_latency_ms INT,
    reranking_latency_ms INT,
    retrieved_chunk_ids UUID[],
    retrieval_scores DOUBLE PRECISION[],
    finish_reason TEXT,
    retry_count INT NOT NULL DEFAULT 0,
    cache_hit BOOLEAN NOT NULL DEFAULT false,
    estimated_cost_usd NUMERIC(12, 8),
    tool_calls JSONB,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_llm_runs_tenant_id ON llm_runs (tenant_id);

CREATE INDEX idx_llm_runs_user_id ON llm_runs (user_id);

CREATE INDEX idx_llm_runs_created_at ON llm_runs (created_at);

CREATE INDEX idx_llm_runs_finish_reason ON llm_runs (finish_reason)
WHERE
    finish_reason != 'STOP';

CREATE TABLE eval_cases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dataset_name TEXT NOT NULL,
    question TEXT NOT NULL,
    expected_answer TEXT,
    expected_chunk_ids UUID[],
    tenant_id UUID REFERENCES tenants (id),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE eval_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dataset_name TEXT NOT NULL,
    prompt_version TEXT,
    model TEXT,
    is_baseline BOOLEAN NOT NULL DEFAULT false,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    avg_precision_at_k DOUBLE PRECISION,
    avg_recall_at_k DOUBLE PRECISION,
    avg_faithfulness DOUBLE PRECISION,
    pass_rate DOUBLE PRECISION
);

CREATE TABLE eval_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    eval_run_id UUID NOT NULL REFERENCES eval_runs (id) ON DELETE CASCADE,
    eval_case_id UUID NOT NULL REFERENCES eval_cases (id) ON DELETE CASCADE,
    retrieved_chunk_ids UUID[],
    generated_answer TEXT,
    precision_at_k DOUBLE PRECISION,
    recall_at_k DOUBLE PRECISION,
    faithfulness_score DOUBLE PRECISION,
    passed BOOLEAN,
    judge_reasoning TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_eval_results_run_id ON eval_results (eval_run_id);