CREATE TABLE document_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    document_id UUID NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    document_version_id UUID NOT NULL REFERENCES document_versions (id) ON DELETE CASCADE,
    parent_chunk_id UUID REFERENCES document_chunks (id) ON DELETE CASCADE,
    chunk_index INT NOT NULL,
    content TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    embedding vector (1536),
    page INT,
    section TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    token_count INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX document_chunks_unique_content_idx ON document_chunks (
    document_version_id,
    COALESCE(
        parent_chunk_id,
        '00000000-0000-0000-0000-000000000000'::uuid
    ),
    content_hash
);

CREATE INDEX idx_document_chunks_tenant_id ON document_chunks (tenant_id);

CREATE INDEX idx_document_chunks_document_version_id ON document_chunks (document_version_id);

CREATE INDEX idx_document_chunks_parent_id ON document_chunks (parent_chunk_id)
WHERE
    parent_chunk_id IS NOT NULL;

CREATE INDEX idx_document_chunks_embedding_hnsw ON document_chunks USING hnsw (embedding vector_cosine_ops)
WITH
    (m = 16, ef_construction = 64)
WHERE
    embedding IS NOT NULL;

ALTER TABLE document_chunks
ADD COLUMN content_tsv tsvector GENERATED ALWAYS AS (to_tsvector('english', content)) STORED;

CREATE INDEX idx_document_chunks_content_tsv ON document_chunks USING gin (content_tsv);