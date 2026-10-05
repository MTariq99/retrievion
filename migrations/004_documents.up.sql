-- 004_documents.up.sql
-- Documents are the tenant-owned "logical" file (e.g. "employee-handbook.pdf").
-- document_versions is what actually gets parsed/chunked/embedded — this
-- separation is what makes document versioning (Topic 8, Section 3.11 /
-- Section 9's "stale document" failure scenario) possible: re-uploading a
-- changed file creates a NEW version rather than mutating history in place.
CREATE TABLE documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    -- Mirrors the architecture doc's explicit document lifecycle states.
    status TEXT NOT NULL DEFAULT 'pending' CHECK (
        status IN (
            'pending',
            'processing',
            'active',
            'failed',
            'archived'
        )
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

CREATE INDEX idx_documents_tenant_id ON documents (tenant_id);

CREATE TRIGGER trg_documents_updated_at
BEFORE UPDATE ON documents FOR EACH ROW
EXECUTE FUNCTION set_updated_at ();

CREATE TABLE document_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    version INT NOT NULL,
    -- SHA256 of the raw uploaded file bytes (Section 51 of the architecture
    -- doc). This is the whole-document analogue of Topic 8's deterministic
    -- chunk ID: re-uploading byte-identical content is detectable BEFORE
    -- spending anything on parsing/chunking/embedding it again.
    content_hash TEXT NOT NULL,
    storage_key TEXT NOT NULL, -- pointer into object storage (S3/MinIO), not the file itself
    status TEXT NOT NULL DEFAULT 'pending' CHECK (
        status IN (
            'pending',
            'processing',
            'active',
            'failed',
            'archived'
        )
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Version numbers are only meaningful scoped to a single document, and
    -- must be gapless/unique per document (version 1, 2, 3 — never two "2"s).
    CONSTRAINT document_versions_unique_version UNIQUE (document_id, version)
);

CREATE INDEX idx_document_versions_document_id ON document_versions (document_id);

-- Enables the exact idempotency check from Section 51: "has this exact file
-- content already been uploaded for this document?" — a single indexed
-- lookup instead of comparing hashes in application code after fetching
-- every prior version.
CREATE INDEX idx_document_versions_content_hash ON document_versions (document_id, content_hash);

-- CRITICAL invariant this schema does NOT yet enforce with a constraint,
-- deliberately left to application logic: "only one version per document
-- may have status = 'active' at a time" (Section 9's "stale document"
-- failure scenario depends on this being true). A partial unique index
-- enforces it cleanly:
CREATE UNIQUE INDEX idx_document_versions_one_active ON document_versions (document_id)
WHERE
    status = 'active';