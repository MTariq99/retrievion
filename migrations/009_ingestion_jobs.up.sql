-- 009_ingestion_jobs.up.sql
-- Note on numbering: the architecture doc lists 008_sources.up.sql as a
-- separate migration. Since citation "sources" are really just
-- message_sources (already created in 007, tightly coupled to messages),
-- that table was folded into 007 rather than split across two files.
-- 009 is kept as-numbered here for continuity with the doc's own scheme.
-- This table is what makes ingestion OBSERVABLE (Section 15 of the
-- architecture doc) and is the concrete implementation of the "check
-- idempotency, retry, dead-letter" worker flow (Section 14) — every job
-- attempt, failure, and retry is tracked here, not just logged and lost.
CREATE TABLE ingestion_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    document_id UUID NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    document_version_id UUID NOT NULL REFERENCES document_versions (id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'queued' CHECK (
        status IN (
            'queued',
            'processing',
            'completed',
            'failed',
            'dead_letter'
        )
    ),
    -- Tracks retry count so the worker can enforce a bounded retry policy
    -- (Section 50: "use bounded retries and exponential backoff", directly
    -- extending Topic 2/4's isRetryable pattern to the ingestion pipeline)
    -- rather than retrying forever.
    attempts INT NOT NULL DEFAULT 0,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_ingestion_jobs_tenant_id ON ingestion_jobs (tenant_id);

CREATE INDEX idx_ingestion_jobs_document_version_id ON ingestion_jobs (document_version_id);

-- The worker's core query is "give me queued jobs to process next" — this
-- partial index keeps that lookup fast regardless of how many completed/
-- failed historical jobs accumulate over time (which will vastly
-- outnumber the currently-queued ones in any long-running system).
CREATE INDEX idx_ingestion_jobs_queued ON ingestion_jobs (created_at)
WHERE
    status = 'queued';