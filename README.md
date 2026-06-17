# motzworks

Agentless network discovery and asset-inventory tool — a modern replacement for
the discontinued Spiceworks Inventory. It scans the network by authenticating to
hosts (no agent installed), normalizes everything into a unified asset database,
serves admin dashboards in the browser, and feeds Zabbix.

See [PLAN.md](PLAN.md) for the full architecture and roadmap.

## Status

**Phase 4 — Hardening (in progress).** Distributed collectors were descoped
(single-scanner deployment), so this phase is security/scale hardening:

- **Audit log** — `audit_log` table recording logins, login failures, logouts
  and admin mutations (credentials/targets/schedules/scan triggers), with
  actor + client IP; secrets are never logged. Admin API: `GET /api/audit`.
- **Scan politeness** — discovery rate limiting (`-rate` / `scan.rate_per_sec`)
  and per-host jitter (`-jitter` / `scan.jitter_ms`) for large networks, applied
  to both ad-hoc and scheduled scans.

**Phase 3 — Integrations.** Added vendor/API collectors and a
Zabbix integration path:

- **Proxmox VE** — inventories the hypervisor and enumerates its VMs/containers
  as child devices linked by a `hosts-vm` relationship.
- **OPNsense** and **FortiGate** — REST/API collectors (firmware, version,
  model, serial), self-identifying so they only claim hosts that match.
- **Zabbix** — stable, denormalized SQL **views** (`zbx_host_inventory`,
  `zbx_device_software`, `zbx_scan_status`) for Zabbix to pull from PostgreSQL
  directly (ODBC "Database monitor" items / SQL data source).
- Collector results can now carry **related devices + relationships**, persisted
  by the engine.

New scan credentials: `-proxmox-token`/`-proxmox-secret`, `-opnsense-key`/
`-opnsense-secret`, `-fortigate-token` (and the matching credential `kind`s in
the vault/API).

> Live-validated so far: discovery, SSH, SNMP, the API, dashboards, scheduler,
> Zabbix views. The Proxmox/OPNsense/FortiGate collectors and WinRM are
> implemented with parser unit tests but await validation against live targets.
> Active Directory (LDAP) is deferred until the firewall opens 389/636 to the DC.

### Phase 2 — Scheduler + API + dashboards

On top of Phase 1, the scanner now
serves a web dashboard and REST API, with authenticated admin access and
recurring scans:

- **Auth + RBAC** — local accounts, bcrypt passwords, HMAC-signed session
  cookies, `admin`/`viewer` roles.
- **REST API** — devices (search/filter/paginate), device detail, software
  rollups, change timeline, scan history, dashboard stats, CSV export.
- **Admin API** — manage scan targets, vault-sealed credentials and schedules;
  trigger ad-hoc scans.
- **Scheduler** — background loop runs due schedules using stored credentials.
- **Dashboard** — embedded React/TS SPA (single binary): dashboard, devices,
  device detail, software, changes, scans, and an admin panel.

```sh
# One-time setup
export MOTZWORKS_VAULT_KEY="$(motzworks vault genkey)"   # keep this stable
export MOTZWORKS_AUTH_SECRET="$(openssl rand -base64 32)" # stable session secret
motzworks migrate up
motzworks user add -username admin -password '<pw>' -role admin

# Run API + scheduler + dashboard
motzworks serve            # http://localhost:8080
```

Building the dashboard from source:

```sh
cd web/ui && npm install && npm run build   # outputs to internal/web/dist
# during UI development: `npm run dev` (proxies /api to :8080), `motzworks serve` separately
```

### Phase 1 recap

On top of the Phase 0 foundation
(config, logging, schema/migrations, vault, worker pool), the scanner now does:

- **Discovery** — expand CIDRs / IPs / ranges, TCP-connect liveness probing, plus
  an SNMP UDP probe to find network-only gear.
- **Fingerprinting** — classify hosts (Windows / Linux / SNMP / hypervisor) from
  open ports.
- **Collectors** — SSH (Linux/Unix), SNMP (network devices), and WinRM (Windows).
- **Engine + persistence** — fan-out collection, normalization, identity dedup
  (serial / AD GUID / MAC / hostname / IP) and field-level change tracking.

Run a scan from the CLI:

```sh
# Discover + inventory a subnet with credentials for each protocol
motzworks scan -targets 10.0.0.0/24 \
  -ssh-user svc-scan -ssh-key ~/.ssh/scan_ed25519 \
  -snmp-community public \
  -winrm-user 'CORP\svc-scan' -winrm-pass '...'

# Discovery only (no credentials), custom ports
motzworks scan -targets 10.0.0.0/24 -ports 22,443,3389,5985
```

Scheduling, the REST API and the web dashboards arrive in Phase 2.

## Quick start (development)

Requires Docker, plus either Nix (recommended) or a local Go 1.26+ toolchain.

### Dev environment via Nix

A flake provides Go, gopls, staticcheck, golangci-lint, delve, Node, and the
`psql` client:

```sh
nix develop          # enter the dev shell
# or, with direnv installed, just `cd` into the repo (.envrc runs `use flake`)
```

### Bring it up

```sh
# 1. Start PostgreSQL
docker compose up -d

# 2. Configure
cp config.example.yaml config.yaml

# 3. Apply the database schema
go run ./cmd/motzworks migrate up

# 4. Generate a vault key (export it for the app to use)
export MOTZWORKS_VAULT_KEY="$(go run ./cmd/motzworks vault genkey)"

# Sanity checks
go run ./cmd/motzworks version
go run ./cmd/motzworks config check
```

## Layout

```
cmd/motzworks/        CLI entrypoint and subcommands
internal/config/      YAML + env configuration
internal/logging/     structured logging (slog)
internal/model/       unified asset domain types
internal/store/       PostgreSQL store + embedded SQL migrations
internal/vault/       credential sealing (NaCl secretbox)
internal/collector/   pluggable collector interface + registry
internal/worker/      bounded-concurrency job runner
```
