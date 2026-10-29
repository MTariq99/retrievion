CREATE TABLE IF NOT EXISTS outbox_messages (
    id uuid PRIMARY KEY,
    kind text NOT NULL,
    payload bytea NOT NULL,
    attempts int NOT NULL DEFAULT 0,
    last_error text,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS outbox_due_idx ON outbox_messages (next_attempt_at);