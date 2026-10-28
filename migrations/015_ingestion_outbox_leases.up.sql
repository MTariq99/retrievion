ALTER TABLE ingestion_outbox
ADD COLUMN locked_until TIMESTAMPTZ,
ADD COLUMN dead_letter_at TIMESTAMPTZ;

CREATE INDEX idx_ingestion_outbox_reclaimable ON ingestion_outbox (locked_until, next_attempt_at, created_at)
WHERE
    published_at IS NULL
    AND dead_letter_at IS NULL;
