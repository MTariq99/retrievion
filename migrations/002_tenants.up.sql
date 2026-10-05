-- 002_tenants.up.sql
-- The root of the multi-tenancy model (Topic 9, Section 3.14). Every other
-- table that stores tenant-owned data carries a tenant_id foreign key back
-- to this table, and every retrieval query MUST filter on it.
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Keep updated_at accurate without relying on application code to remember
-- to set it on every UPDATE — a trigger is the reliable, structural way to
-- guarantee this (same philosophy as Section 3.14: don't trust every call
-- site to do the right thing, enforce it centrally).
CREATE OR REPLACE FUNCTION set_updated_at () RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_tenants_updated_at
BEFORE UPDATE ON tenants FOR EACH ROW
EXECUTE FUNCTION set_updated_at ();