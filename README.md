# motzworks

Agentless network discovery and asset-inventory tool — a modern replacement for
the discontinued Spiceworks Inventory. It scans the network by authenticating to
hosts (no agent installed), normalizes everything into a unified asset database,
serves admin dashboards in the browser, and feeds Zabbix.

See [PLAN.md](PLAN.md) for the full architecture and roadmap.

## Status

**Phase 1 — Discovery + core collectors.** On top of the Phase 0 foundation
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
