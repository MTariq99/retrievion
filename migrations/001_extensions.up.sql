-- 001_extensions.up.sql
-- Enables the Postgres extensions the whole schema depends on.
-- pgvector: adds the `vector` column type, distance operators (<->, <=>, <#>),
-- and HNSW/IVFFlat index types. This is the foundation for all semantic search
-- in this project (Topic 9).
CREATE EXTENSION IF NOT EXISTS vector;

-- pgcrypto: provides gen_random_uuid() for primary keys. (Postgres 13+ also
-- exposes gen_random_uuid() natively, but pgcrypto is kept explicit here so
-- the migration works identically across Postgres versions without relying
-- on which core version first added it.)
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- pg_trgm: enables trigram-based fuzzy text matching, useful later for
-- typo-tolerant keyword search / ILIKE performance on chunk content.
-- Not required for MVP hybrid search (native tsvector/FTS covers that),
-- but cheap to enable now rather than as a disruptive later migration.
CREATE EXTENSION IF NOT EXISTS pg_trgm;