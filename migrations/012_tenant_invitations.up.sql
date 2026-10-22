CREATE TABLE tenant_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'member', 'viewer')),
    token_hash BYTEA NOT NULL UNIQUE,
    invited_by_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'revoked')),
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_by_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    accepted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT tenant_invitations_email_nonempty CHECK (length(trim(email)) > 0),
    CONSTRAINT tenant_invitations_acceptance_consistent CHECK (
        (
            status = 'accepted'
            AND accepted_by_user_id IS NOT NULL
            AND accepted_at IS NOT NULL
        )
        OR (
            status <> 'accepted'
            AND accepted_by_user_id IS NULL
            AND accepted_at IS NULL
        )
    )
);

CREATE UNIQUE INDEX tenant_invitations_one_pending_per_email ON tenant_invitations (tenant_id, lower(email))
WHERE
    status = 'pending';

CREATE INDEX tenant_invitations_tenant_status_idx ON tenant_invitations (tenant_id, status);
