-- 005_chunks.up.sql
-- The heart of the whole system. This table stores every chunk (both
-- parent AND child, per Topic 8's parent-child pattern — Section 19 of the
-- architecture doc), its embedding vector, and enough metadata to trace it
-- back to an exact source location and to filter it by tenant.
--
-- IMPORTANT: pick your embedding model BEFORE running this migration.
-- The vector column below is fixed at 3072 dimensions to match
-- gemini-embedding-001 (Topic 7). If you change embedding providers later,
-- per Topic 9 Section 3.17, this is a real schema change (ALTER COLUMN
-- TYPE + full re-embedding + index rebuild), not just new data in the
-- same shape — plan for that explicitly rather than discovering it later.
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

-- Tenant filtering happens on EVERY retrieval query without exception
-- (Section 3.14) — this index is not optional.
CREATE INDEX idx_document_chunks_tenant_id ON document_chunks (tenant_id);

CREATE INDEX idx_document_chunks_document_version_id ON document_chunks (document_version_id);

-- Supports "given a matched child, fetch its parent" (Section 19's
-- retrieval-time context expansion) as a single indexed lookup.
CREATE INDEX idx_document_chunks_parent_id ON document_chunks (parent_chunk_id)
WHERE
    parent_chunk_id IS NOT NULL;

-- The HNSW vector index (Topic 9, Section 3.4). vector_cosine_ops MUST
-- match whatever distance operator your queries actually use (<=> for
-- cosine) — a mismatch here is a silent, no-error correctness bug
-- (Section 3.18). m and ef_construction are reasonable starting defaults,
-- NOT tuned values — revisit these against real recall/latency
-- measurements once you have production-like data volume and query
-- patterns (Section 8's tuning discussion).
--
-- Only child chunks have non-NULL embeddings, so this partial index avoids
-- wasting index space on parent rows that will never be searched directly.
CREATE INDEX idx_document_chunks_embedding_hnsw ON document_chunks USING hnsw (embedding vector_cosine_ops)
WITH
    (m = 16, ef_construction = 64)
WHERE
    embedding IS NOT NULL;

-- Full-text search index (Section 24: PostgreSQL FTS for the keyword half
-- of hybrid search — error codes, names, exact phrases that embeddings
-- alone can under-rank, per Topic 9 Section 3.12). GIN over a generated
-- tsvector column is the standard, efficient pattern — computing
-- to_tsvector() at query time on every row would be far slower than
-- indexing a precomputed column.
ALTER TABLE document_chunks
ADD COLUMN content_tsv tsvector GENERATED ALWAYS AS (to_tsvector('english', content)) STORED;

CREATE INDEX idx_document_chunks_content_tsv ON document_chunks USING gin (content_tsv);