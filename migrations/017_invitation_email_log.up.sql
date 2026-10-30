CREATE TABLE IF NOT EXISTS email_invitation_log (
    message_id uuid PRIMARY KEY,
    invitation_id uuid NOT NULL REFERENCES tenant_invitations (id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'sending' CHECK (status IN ('sending', 'sent', 'failed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_invitation_log_invitation ON email_invitation_log (invitation_id);