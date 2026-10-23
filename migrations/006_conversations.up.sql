CREATE TABLE conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title TEXT,
    summary TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_conversations_tenant_id ON conversations (tenant_id);

CREATE INDEX idx_conversations_user_id ON conversations (user_id);

CREATE TRIGGER trg_conversations_updated_at
BEFORE UPDATE ON conversations FOR EACH ROW
EXECUTE FUNCTION set_updated_at ();

CREATE TABLE messages (
    id UUID PRIMARY KEY,
    conversation_id UUID NOT NULL,
    role VARCHAR(20) NOT NULL,
    content TEXT NOT NULL,
    token_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT messages_conversation_fk FOREIGN KEY (conversation_id) REFERENCES conversations (id) ON DELETE CASCADE,
    CONSTRAINT messages_role_check CHECK (role IN ('system', 'user', 'assistant'))
);

CREATE INDEX idx_messages_conversation_created_at ON messages (conversation_id, created_at DESC);

CREATE TABLE IF NOT EXISTS message_sources (
    id UUID PRIMARY KEY,
    message_id UUID NOT NULL,
    chunk_id UUID NOT NULL,
    document_id UUID NOT NULL,
    citation_index INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_message_sources_message FOREIGN KEY (message_id) REFERENCES messages (id) ON DELETE CASCADE,
    CONSTRAINT fk_message_sources_chunk FOREIGN KEY (chunk_id) REFERENCES document_chunks (id) ON DELETE CASCADE,
    CONSTRAINT fk_message_sources_document FOREIGN KEY (document_id) REFERENCES documents (id) ON DELETE CASCADE,
    CONSTRAINT uq_message_source UNIQUE (message_id, chunk_id)
);

CREATE INDEX IF NOT EXISTS idx_message_sources_message_id ON message_sources (message_id);

CREATE INDEX IF NOT EXISTS idx_message_sources_chunk_id ON message_sources (chunk_id);

CREATE INDEX IF NOT EXISTS idx_message_sources_document_id ON message_sources (document_id);