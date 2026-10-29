CREATE TABLE IF NOT EXISTS outbox_messages (
    id uuid PRIMARY KEY,
    kind text not null,
    payload bytea not null,
    attempts int not null DEFAULT 0,
    last_err text,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
)
CREATE INDEX outbox_due_idx ON outbox_messages (next_attempt_at);