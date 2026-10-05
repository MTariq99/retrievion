-- 003_users.up.sql
-- Users are NOT tenant-scoped themselves (one user can belong to multiple
-- tenants — e.g. a consultant working across several client workspaces).
-- Membership, and therefore access, is expressed through tenant_members.
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    name TEXT NOT NULL,
    role TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ DEFAULT NULL,
    -- Emails must be globally unique across the whole system (login is by
    -- email, not by email+tenant) — this is a deliberate product decision,
    -- not an accident. Case-insensitive uniqueness via lower(email) avoids
    -- "Bob@x.com" and "bob@x.com" being treated as different accounts.
    CONSTRAINT users_email_unique UNIQUE (email)
);

CREATE UNIQUE INDEX idx_users_email_lower ON users (lower(email));

CREATE TRIGGER trg_users_updated_at
BEFORE UPDATE ON users FOR EACH ROW
EXECUTE FUNCTION set_updated_at ();

-- tenant_members is the actual authorization join table (Section 8 of the
-- architecture doc: "user belongs to tenant" must be true before any
-- access is granted — this table is what middleware/tenant.go checks).
CREATE TABLE tenant_members (
    tenant_id UUID NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id),
    -- CHECK over a Postgres ENUM type deliberately: adding a new role later
    -- is a simple constraint change, not the more disruptive
    -- ALTER TYPE ... ADD VALUE dance that native enums require mid-transaction.
    CONSTRAINT tenant_members_role_check CHECK (role IN ('owner', 'admin', 'member', 'viewer'))
);

-- Reverse lookup: "which tenants does this user belong to" (used at login
-- to resolve available workspaces) needs its own index — the primary key
-- above only optimizes "given a tenant, list its members" efficiently.
CREATE INDEX idx_tenant_members_user_id ON tenant_members (user_id);