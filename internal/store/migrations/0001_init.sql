-- 0001_init.sql — core asset inventory schema.
-- Targets PostgreSQL 14+ (gen_random_uuid, inet, macaddr, jsonb are built in).

-- Devices: the central asset record. serial / ad_guid / hostname / interface
-- MACs are the identity keys used to dedup a device seen via multiple sources.
CREATE TABLE device (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_type TEXT NOT NULL DEFAULT 'unknown',
    hostname    TEXT,
    primary_ip  INET,
    serial      TEXT,
    asset_tag   TEXT,
    ad_guid     TEXT,
    source      TEXT,
    first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_device_hostname ON device (lower(hostname));
CREATE INDEX idx_device_primary_ip ON device (primary_ip);
CREATE UNIQUE INDEX idx_device_serial ON device (serial) WHERE serial IS NOT NULL;
CREATE UNIQUE INDEX idx_device_ad_guid ON device (ad_guid) WHERE ad_guid IS NOT NULL;

-- Network interfaces.
CREATE TABLE interface (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id  UUID NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    name       TEXT,
    mac        MACADDR,
    ip         INET,
    speed_mbps BIGINT,
    vlan       INTEGER
);
CREATE INDEX idx_interface_device ON interface (device_id);
CREATE INDEX idx_interface_mac ON interface (mac);

-- Operating system facts (one current row per device).
CREATE TABLE os_info (
    device_id UUID PRIMARY KEY REFERENCES device(id) ON DELETE CASCADE,
    family    TEXT,   -- windows, linux, macos, ...
    name      TEXT,
    version   TEXT,
    build     TEXT,
    arch      TEXT
);

-- Hardware facts (one row per device).
CREATE TABLE hardware (
    device_id UUID PRIMARY KEY REFERENCES device(id) ON DELETE CASCADE,
    vendor    TEXT,
    model     TEXT,
    serial    TEXT,
    cpu       TEXT,
    cpu_cores INTEGER,
    ram_bytes BIGINT
);

-- Installed software (many per device).
CREATE TABLE software (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id    UUID NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    version      TEXT,
    vendor       TEXT,
    install_date DATE
);
CREATE INDEX idx_software_device ON software (device_id);
CREATE INDEX idx_software_name ON software (lower(name));

-- User accounts discovered on devices or in the directory.
CREATE TABLE user_account (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id  UUID REFERENCES device(id) ON DELETE CASCADE,
    username   TEXT NOT NULL,
    full_name  TEXT,
    last_logon TIMESTAMPTZ,
    is_local   BOOLEAN NOT NULL DEFAULT true
);
CREATE INDEX idx_user_account_device ON user_account (device_id);

-- Relationships between devices (e.g. hypervisor -> vm, host -> ad).
CREATE TABLE relationship (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id UUID NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    child_id  UUID NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    kind      TEXT NOT NULL,   -- hosts-vm, member-of-ad, ...
    UNIQUE (parent_id, child_id, kind)
);

-- Credentials. The secret is sealed by the vault; the key is never stored here.
CREATE TABLE credential (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL,   -- ssh-password, ssh-key, winrm, snmp-v2c, snmp-v3, api-token
    username      TEXT,
    secret_sealed TEXT,            -- base64(nonce||ciphertext)
    extra         JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Scan scopes: CIDR/host ranges to discover and inventory.
CREATE TABLE scan_target (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    cidrs      TEXT[] NOT NULL DEFAULT '{}',
    enabled    BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Schedules drive recurring scans of a target.
CREATE TABLE schedule (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scan_target_id UUID NOT NULL REFERENCES scan_target(id) ON DELETE CASCADE,
    interval_secs  INTEGER NOT NULL,
    enabled        BOOLEAN NOT NULL DEFAULT true,
    next_run_at    TIMESTAMPTZ
);

-- Scan runs record each execution.
CREATE TABLE scan_run (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scan_target_id UUID REFERENCES scan_target(id) ON DELETE SET NULL,
    started_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at    TIMESTAMPTZ,
    status         TEXT NOT NULL DEFAULT 'running',   -- running, ok, failed
    hosts_found    INTEGER NOT NULL DEFAULT 0,
    error          TEXT
);

-- Change timeline: field-level diffs across scans.
CREATE TABLE change_event (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id   UUID NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    scan_run_id UUID REFERENCES scan_run(id) ON DELETE SET NULL,
    field       TEXT NOT NULL,
    old_value   TEXT,
    new_value   TEXT,
    ts          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_change_event_device ON change_event (device_id, ts DESC);

-- Raw collector payloads for audit/debug.
CREATE TABLE raw_payload (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scan_run_id UUID REFERENCES scan_run(id) ON DELETE CASCADE,
    device_id   UUID REFERENCES device(id) ON DELETE CASCADE,
    collector   TEXT NOT NULL,
    payload     JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
