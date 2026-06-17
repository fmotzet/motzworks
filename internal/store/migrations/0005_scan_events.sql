-- 0005_scan_events.sql — per-host progress events for live scan monitoring.

ALTER TABLE scan_run ADD COLUMN discovered INTEGER NOT NULL DEFAULT 0;

CREATE TABLE scan_event (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scan_run_id  UUID NOT NULL REFERENCES scan_run(id) ON DELETE CASCADE,
    ts           TIMESTAMPTZ NOT NULL DEFAULT now(),
    addr         TEXT NOT NULL,
    device_class TEXT,
    collector    TEXT,
    status       TEXT NOT NULL,          -- collected | discovered | failed
    changes      INTEGER NOT NULL DEFAULT 0,
    error        TEXT
);
CREATE INDEX idx_scan_event_run ON scan_event (scan_run_id, ts);
