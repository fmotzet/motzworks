-- 0004_audit.sql — audit trail of authentication and administrative actions.

CREATE TABLE audit_log (
    id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ts      TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor   TEXT,                       -- username, or '' for anonymous
    action  TEXT NOT NULL,              -- login, login_failed, logout, create_credential, ...
    target  TEXT,                       -- what was acted on (name/id)
    ip      TEXT,                        -- client IP
    detail  JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX idx_audit_log_ts ON audit_log (ts DESC);
CREATE INDEX idx_audit_log_actor ON audit_log (actor);
