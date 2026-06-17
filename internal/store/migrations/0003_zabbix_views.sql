-- 0003_zabbix_views.sql — stable, denormalized read views for Zabbix.
-- Zabbix pulls inventory directly from PostgreSQL (e.g. ODBC "Database monitor"
-- items or a SQL data source). These views give it a flat, stable contract that
-- won't shift as the internal schema evolves.

-- One row per device, mapped toward Zabbix host-inventory concepts.
CREATE VIEW zbx_host_inventory AS
SELECT
    d.id::text                                          AS device_id,
    COALESCE(NULLIF(d.hostname, ''), host(d.primary_ip)) AS name,
    d.device_type                                       AS type,
    host(d.primary_ip)                                  AS primary_ip,
    COALESCE(o.name, '')                                AS os,
    btrim(COALESCE(o.name, '') || ' ' || COALESCE(o.version, '') || ' ' || COALESCE(o.build, '')) AS os_full,
    COALESCE(o.arch, '')                                AS arch,
    COALESCE(h.vendor, '')                              AS hw_vendor,
    COALESCE(h.model, '')                               AS hw_model,
    COALESCE(NULLIF(d.serial, ''), h.serial, '')        AS serial,
    COALESCE(h.cpu, '')                                 AS cpu,
    COALESCE(h.cpu_cores, 0)                            AS cpu_cores,
    COALESCE(h.ram_bytes, 0)                            AS ram_bytes,
    COALESCE(d.source, '')                              AS source,
    d.first_seen,
    d.last_seen,
    (SELECT count(*) FROM software s WHERE s.device_id = d.id)                                  AS software_count,
    (SELECT string_agg(DISTINCT i.mac::text, ',') FROM interface i
       WHERE i.device_id = d.id AND i.mac IS NOT NULL)                                          AS macs
FROM device d
LEFT JOIN os_info o ON o.device_id = d.id
LEFT JOIN hardware h ON h.device_id = d.id;

-- One row per installed software title per device.
CREATE VIEW zbx_device_software AS
SELECT
    d.id::text                                          AS device_id,
    COALESCE(NULLIF(d.hostname, ''), host(d.primary_ip)) AS name,
    s.name                                              AS software,
    COALESCE(s.version, '')                             AS version,
    COALESCE(s.vendor, '')                              AS vendor
FROM software s
JOIN device d ON d.id = s.device_id;

-- Most recent scan run, for a Zabbix "freshness" / heartbeat item.
CREATE VIEW zbx_scan_status AS
SELECT
    id::text                                            AS scan_run_id,
    status,
    hosts_found,
    started_at,
    finished_at,
    EXTRACT(EPOCH FROM (now() - started_at))::bigint    AS seconds_since_start
FROM scan_run
ORDER BY started_at DESC
LIMIT 1;
