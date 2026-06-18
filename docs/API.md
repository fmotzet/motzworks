# motzworks API

JSON REST API served alongside the dashboard. Authentication is a session cookie
(`mw_session`) issued by `POST /api/login`; send it on subsequent requests.
Roles: **viewer** (read) and **admin** (read + manage). Unauthenticated requests
to protected routes get `401`; viewers hitting admin routes get `403`.

## Auth

| Method | Path | Role | Body / notes |
|---|---|---|---|
| POST | `/api/login` | public | `{"username","password"}` → sets cookie, returns `{username,role}` |
| POST | `/api/logout` | public | clears the cookie |
| GET | `/api/me` | viewer | current `{username,role}` |
| GET | `/api/health` | public | `{"status":"ok"}` |

## Inventory (viewer)

| Method | Path | Notes |
|---|---|---|
| GET | `/api/stats` | dashboard summary (totals, by-type, last scan) |
| GET | `/api/devices` | query: `q`, `type`, `limit`, `offset` → `{items,total,limit,offset}` |
| GET | `/api/devices/{id}` | full device detail (OS, hardware, interfaces, software, users) |
| GET | `/api/devices.csv` | CSV export (honors `q`, `type`) |
| GET | `/api/software` | software rollup; query: `q`, `limit` |
| GET | `/api/changes` | change timeline; query: `device_id`, `limit` |
| GET | `/api/scans` | recent scan runs; query: `limit` |
| GET | `/api/scans/{id}` | one run + its per-host events (`{scan, events}`); poll while `status=running` for live progress |
| GET | `/api/targets` | scan targets |
| GET | `/api/schedules` | schedules |

## Management (admin)

| Method | Path | Body / notes |
|---|---|---|
| POST | `/api/scans` | `{"targets":[...]} ` or `{"target_id":"..."}` → 202, runs in background |
| POST | `/api/targets` | `{"name","cidrs":[...],"enabled"?}` |
| DELETE | `/api/targets/{id}` | |
| GET/POST | `/api/credentials` | POST `{"name","kind","username","secret","extra"?}` — secret is vault-sealed; never returned |
| DELETE | `/api/credentials/{id}` | |
| GET/POST | `/api/schedules` | POST `{"scan_target_id","interval_secs"(>=60),"enabled"?}` |
| GET | `/api/audit` | audit log; query: `limit` |

Credential `kind` values: `ssh-password`, `ssh-key`, `wmi` (Windows via
WMI/DCOM — `username` may be `DOMAIN\user` / `user@domain`, optional
`extra.domain`), `winrm`, `snmp-v2c`, `snmp-v3`, `proxmox-token`, `opnsense-api`,
`fortigate-token`, `api-token`.

## Observability

| Method | Path | Notes |
|---|---|---|
| GET | `/metrics` | Prometheus exposition (unauthenticated; aggregate counts) |
