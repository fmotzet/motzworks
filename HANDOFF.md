# Session handoff

Working notes for picking up motzworks on another machine / in a fresh Claude
session. Delete or trim freely — this is a scratch handoff, not permanent docs.

## State

All code is committed + pushed to `origin/master`. Phases 0–5 of [PLAN.md](PLAN.md)
are implemented. See the README "Status" section for the feature list.

**Live-verified:** discovery, SSH (incl. a NixOS software profile), SNMP, the
REST API + React dashboard + auth/RBAC, scheduler, audit log, Zabbix views,
Docker image, Prometheus `/metrics`, live scan-detail view, collapsible/search
device sections, scan-target recording.

**Implemented but NOT yet validated against a live target** (parsers are
unit-tested; need a host + API token each): Proxmox, OPNsense, FortiGate, and
WinRM (WinRM works fine over NTLM on ordinary member servers — only the test DC
is a problem; see below).

**Not built yet:** Active Directory / LDAP collector (deferred — blocked on
firewall, see below). Optional Phase-4 leftovers: change-alerting webhooks.

## Blocked on infrastructure (not code)

- **Windows test DC `bgdc1po-adts` / `10.20.30.70`** (domain `ad.boerse-go.de`,
  NetBIOS `AD`): its WinRM listener is **Kerberos-only** — NTLM is conclusively
  rejected (verified across go-ntlmssp, requests_ntlm, pyspnego), even with
  incoming-NTLM allowed and the account in `Remote Management Users` +
  `WinRMRemoteWMIUsers__`. The firewall to that DC also blocks 53/88/389/636.
  To inventory it: open **88/tcp+udp + 53**, implement **Kerberos auth**
  (gokrb5 + a SPNEGO transport for masterzen/winrm), and target it by
  hostname (Kerberos is SPN-based, not IP). Same ports unblock the future
  AD/LDAP collector (needs 389/636). The scanning account `ldap-readonly` is
  ready on the WMI/Remote-Management side.

## Dev environment on a fresh machine

```sh
# tooling comes from the flake; docker is host-installed
nix develop
docker compose up -d                                   # Postgres on :5432

export MOTZWORKS_VAULT_KEY="$(go run ./cmd/motzworks vault genkey)"  # KEEP STABLE
export MOTZWORKS_AUTH_SECRET="$(openssl rand -base64 32)"            # KEEP STABLE
go run ./cmd/motzworks migrate up
go run ./cmd/motzworks user add -username admin -password '<pw>' -role admin
go run ./cmd/motzworks serve                           # http://localhost:8080
```

UI dev: `cd web/ui && npm run dev` (→ :5173, proxies `/api` to :8080).
After UI edits run `npm run build` (writes the embedded `internal/web/dist`).

## Conventions / gotchas (these are in local ~/.claude memory, repeated here)

- **OS edge cases**: add `internal/collector/ssh/profile_<os>.go` implementing
  the `Profile` interface; the generic profile covers mainstream Linux. Vendor
  appliances are separate collector packages. (First profile: `profile_nixos.go`.)
- **Tooling**: this is a Nix box — use `nix-shell -p <pkg> --run '…'` for
  anything not in the flake. Docker is installed.
- **Commits**: the user commits/pushes themselves — don't commit on their behalf.
- **Backend is compiled**: after any Go change, `go build` + restart `serve`.
  The UI hot-reloads under `npm run dev`; the backend does not. A 404 on a new
  API route is almost always a stale `serve` process.
- **Go marshals empty slices as `null`** — store read queries return `[]`
  explicitly and the UI guards with `?? []`.
- A discovery-only / failed scan must never wipe previously-collected child
  data — `UpsertDevice` takes a `collected` flag; regression test
  `TestDiscoveryOnlyPreservesChildren`.
- Store tests need Postgres; they create a throwaway `motzworks_test` DB and
  skip if it's unreachable.

## Known small quirks to revisit

- NixOS user counts are inflated by the 32 `nixbld*` build accounts (a nixos
  profile Users-override could filter them).
- SSH key creds can't be pasted into the dashboard (single-line secret field) —
  a textarea on the credential form would fix it; use the CLI `-ssh-key` for now.
