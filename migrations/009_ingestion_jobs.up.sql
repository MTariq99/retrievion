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
    attempts INT NOT NULL DEFAULT 0,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_ingestion_jobs_tenant_id ON ingestion_jobs (tenant_id);

CREATE INDEX idx_ingestion_jobs_document_version_id ON ingestion_jobs (document_version_id);

CREATE INDEX idx_ingestion_jobs_queued ON ingestion_jobs (created_at)
WHERE
    status = 'queued';