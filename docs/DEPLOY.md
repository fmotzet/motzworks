# Deploying motzworks

motzworks ships as a single static binary with the dashboard embedded, plus a
PostgreSQL database. The recommended deployment is Docker Compose.

## Requirements

- Docker + Docker Compose (or a Go 1.26 binary + PostgreSQL 14+).
- Network reachability from the scanner host to the devices you want to
  inventory (SSH/22, WinRM/5985, SNMP/161, vendor API ports; Kerberos 88 + DNS
  53 if scanning Kerberos-only Windows hosts; LDAP 389/636 for Active Directory).

## Quick start (Docker Compose)

```sh
# Generate and KEEP these stable — rotating the vault key makes stored
# credentials undecryptable; rotating the auth secret logs everyone out.
export MOTZWORKS_VAULT_KEY="$(docker run --rm motzworks:latest vault genkey)"
export MOTZWORKS_AUTH_SECRET="$(openssl rand -base64 32)"
export MOTZWORKS_DB_PASSWORD="$(openssl rand -base64 24)"

docker compose -f docker-compose.prod.yml up -d --build

# Create the first admin user
docker compose -f docker-compose.prod.yml run --rm app \
  user add -username admin -password '<password>' -role admin
```

The dashboard and API are then on `http://<host>:8080`. The container runs
`serve -migrate`, so schema migrations are applied automatically on start.

Put a TLS-terminating reverse proxy (nginx/Caddy/Traefik) in front for HTTPS;
it should forward `X-Forwarded-For` so the audit log records real client IPs.

## Configuration

Everything can be set via environment variables (no config file needed in a
container) or via `config.yaml` (see `config.example.yaml`).

| Variable | Purpose |
|---|---|
| `MOTZWORKS_DB_HOST` / `_PORT` / `_USER` / `_PASSWORD` / `_NAME` / `_SSLMODE` | PostgreSQL connection |
| `MOTZWORKS_VAULT_KEY` | base64 32-byte key that seals stored credentials — **must stay constant** |
| `MOTZWORKS_AUTH_SECRET` | signs session cookies — keep constant or sessions reset |
| `MOTZWORKS_LOG_LEVEL` | `debug` / `info` / `warn` / `error` |

Config-file-only knobs: `server.addr`, `scan.concurrency`, `scan.rate_per_sec`,
`scan.jitter_ms`, `auth.session_hours`.

## First scan

Use the dashboard's **Admin** panel (or the API) to add credentials, scan
targets and schedules, or run an ad-hoc scan. From the CLI:

```sh
docker compose -f docker-compose.prod.yml run --rm app scan \
  -targets 10.0.0.0/24 -ssh-user svc-scan -ssh-key /keys/scan -snmp-community public
```

## Windows (WMI/DCOM)

The Windows collector inventories hosts **agentlessly via WMI over DCOM** (NTLM,
port 135 + dynamic RPC) — the same approach Spiceworks used, and the primary
Windows path (it works even where WinRM is Kerberos-only). It shells out to an
embedded impacket sidecar, so the runtime needs **Python 3 + impacket** — the
provided Docker image already includes them. (Override the interpreter with
`MOTZWORKS_PYTHON` if needed.)

Per Windows host: open the firewall for **135/tcp + the dynamic RPC range** from
the scanner, and use a credential of kind `wmi` (a least-privilege account with
DCOM "Remote Activation" + Remote-Enable on the `root\cimv2` WMI namespace, e.g.
via the "Distributed COM Users" group). Software inventory comes from the
registry Uninstall keys via `StdRegProv`.

## Integrations

- **Zabbix** pulls inventory directly from PostgreSQL via the stable views
  `zbx_host_inventory`, `zbx_device_software`, `zbx_scan_status` (ODBC
  "Database monitor" items or a SQL data source).
- **Prometheus** scrapes `GET /metrics` (unauthenticated; aggregate counts only).

## Backups

The database holds everything (inventory, sealed credentials, users, audit log).

```sh
docker compose -f docker-compose.prod.yml exec postgres \
  pg_dump -U motzworks motzworks | gzip > motzworks-$(date +%F).sql.gz
```

Back up `MOTZWORKS_VAULT_KEY` separately and securely — without it the sealed
credentials in a database dump cannot be decrypted.

## Upgrades

Pull/rebuild the image and recreate the `app` container; `serve -migrate`
applies any new migrations on startup. Take a database backup first.
