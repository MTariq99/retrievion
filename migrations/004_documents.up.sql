CREATE TABLE documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    mime_type TEXT NOT NULL,
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
    content_hash TEXT NOT NULL,
    storage_key TEXT NOT NULL,
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
    CONSTRAINT document_versions_unique_version UNIQUE (document_id, version)
);

CREATE INDEX idx_document_versions_document_id ON document_versions (document_id);

CREATE INDEX idx_document_versions_content_hash ON document_versions (document_id, content_hash);

CREATE UNIQUE INDEX idx_document_versions_one_active ON document_versions (document_id)
WHERE
    status = 'active';