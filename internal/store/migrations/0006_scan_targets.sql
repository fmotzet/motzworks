-- 0006_scan_targets.sql — record what each scan actually targeted.

ALTER TABLE scan_run ADD COLUMN targets TEXT[] NOT NULL DEFAULT '{}';
