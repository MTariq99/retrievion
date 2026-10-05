CREATE TABLE service_clients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id TEXT UNIQUE NOT NULL,
    secret_hash TEXT NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    name TEXT NOT NULL, -- "resolve-agent-orchestrator"
    scopes TEXT[] NOT NULL DEFAULT '{}',
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);