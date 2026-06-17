# motzworks

Agentless network discovery and asset-inventory tool — a modern replacement for
the discontinued Spiceworks Inventory. It scans the network by authenticating to
hosts (no agent installed), normalizes everything into a unified asset database,
serves admin dashboards in the browser, and feeds Zabbix.

See [PLAN.md](PLAN.md) for the full architecture and roadmap.

## Status

**Phase 0 — Foundations.** Project scaffold, config, logging, PostgreSQL schema +
migrations, asset data model, credential vault, collector interface, worker pool,
and a basic CLI. Discovery and collectors land in Phase 1.

## Quick start (development)

Requires Go 1.26+ and Docker.

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
