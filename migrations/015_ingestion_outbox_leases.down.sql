DROP INDEX IF EXISTS idx_ingestion_outbox_reclaimable;

ALTER TABLE ingestion_outbox
DROP COLUMN IF EXISTS dead_letter_at,
DROP COLUMN IF EXISTS locked_until;
