# Network Inventory Scanner — Project Plan

A modern, agentless network discovery and asset-inventory tool to replace the discontinued
Spiceworks Inventory. Scans the network by authenticating to hosts (no agent installed on
clients), normalizes everything into a unified asset database, serves admin dashboards in the
browser, and feeds Zabbix.

> Status: **DRAFT for approval.** Edit anything inline; open questions are marked at the bottom.

---

## 1. Goals & non-goals

**Goals**
- Agentless discovery + inventory of a large, mixed environment (2,500–20,000+ devices).
- Cover: Windows clients/servers, Linux/Mac, SNMP network gear (printers/switches/routers/APs/UPS),
  Active Directory, hypervisors + their VMs, and OPNsense / FortiGate firewalls.
- Collect OS + version, hardware, installed software, users/logins, IPs/MACs/interfaces,
  firmware, serials, and device relationships (VM → hypervisor, host → AD).
- Scheduled recurring rescans with change tracking (first-seen / last-seen / diffs).
- Outputs: **feed Zabbix** (host + inventory sync) and **own browser dashboards** for admins.
- No agent on endpoints; no end-user interaction.

**Non-goals (initially)**
- Full monitoring/alerting platform (Zabbix already does that — we feed it).
- Remote control / patch management / software deployment.
- Public/multi-tenant SaaS.

---

## 2. Language & stack

**Language: Go.**
- Single static cross-platform binary, trivial deployment (Linux-first, also Win/Mac).
- Goroutines = clean high-concurrency scanning of 2k hosts.
- Mature libraries for every protocol we need (see below).

**Stack**
| Concern | Choice | Notes |
|---|---|---|
| Language | Go | core engine, collectors, API |
| Database | PostgreSQL (JSONB) | scale + history; raw collector payloads in JSONB |
| Time-series (opt.) | TimescaleDB | for trend metrics if wanted |
| Web API | Go + chi/echo | REST/JSON |
| Frontend | React + TypeScript (SPA), embedded via `go:embed` | keeps single-binary deploy; charts for dashboards |
| Config | YAML/TOML + env | |
| Secrets | encrypted credential vault (NaCl/age), key from env/KMS | |
| Dev/deploy | Docker Compose (dev), container/systemd (prod) | |

**Key Go libraries**
- SNMP: `gosnmp/gosnmp` (v2c + v3)
- SSH: `golang.org/x/crypto/ssh`
- WinRM: `masterzen/winrm` (PowerShell remoting over HTTP/HTTPS)
- SMB (registry/software fallback): `cloudsoda/go-smb2`
- LDAP/AD: `go-ldap/ldap`
- VMware: `vmware/govmomi` (vCenter/ESXi)
- HTTP vendor APIs: stdlib `net/http` (OPNsense, FortiGate, Proxmox)
- DB: `pgx` + `golang-migrate`

---

## 3. Architecture

```
 Seeds (CIDRs, AD, hypervisors, DNS/DHCP)
        │
        ▼
 ┌─────────────┐   ┌──────────────┐   ┌──────────────────────┐
 │  Discovery  │──▶│ Fingerprint  │──▶│  Collectors (plugins) │
 │ ping/ARP/   │   │ classify     │   │  Win / Linux / SNMP / │
 │ TCP/SNMP    │   │ device type  │   │  VMware / OPNsense /   │
 └─────────────┘   └──────────────┘   │  FortiGate / AD ...    │
                                       └───────────┬───────────┘
                                                   ▼
                              ┌─────────────────────────────────┐
                              │ Normalize + identity/dedup        │
                              │ (merge by MAC/serial/host/AD GUID)│
                              └───────────────┬──────────────────┘
                                              ▼
                                       PostgreSQL (assets + history)
                                              │
                ┌─────────────────────────────┼───────────────────────────┐
                ▼                             ▼                             ▼
        Zabbix sync (API/trapper)     Web dashboards + REST API      Exports (CSV/JSON/webhook)
```

**Components**
1. **Discovery** — seeds from CIDR ranges, AD/LDAP computers, hypervisor inventories, DNS/DHCP.
   Techniques: ICMP sweep, ARP (local), TCP connect probes on common ports, SNMP/NetBIOS/mDNS.
2. **Fingerprinting** — classify each candidate (Windows / Linux / printer / switch / firewall /
   hypervisor) from open ports, SNMP sysObjectID, banners, TTL → picks the right collector.
3. **Collectors** — pluggable modules implementing one interface:
   - **Windows** — WinRM primary (OS, hotfixes, hardware, services, local users, logged-on user;
     installed software via registry Uninstall keys); SMB/registry as fallback.
   - **Linux/Mac** — SSH (uname/distro, dpkg/rpm/brew packages, lshw/dmidecode, users, services).
   - **SNMP** — sysDescr/sysObjectID, interfaces, Entity-MIB (model/serial/firmware), Printer-MIB
     (toner/page counts); v3 preferred.
   - **VMware** — govmomi → hosts, VMs, datastores, allocation, guest info.
   - **Proxmox / Hyper-V** — Proxmox API; Hyper-V via WinRM on the host (if in scope).
   - **OPNsense** — REST API: version, interfaces, packages, firmware, system info.
   - **FortiGate** — FortiOS REST API: model, firmware, HA, interfaces, license, system info.
   - **Active Directory** — LDAP: computers/users/OUs/last-logon; also feeds Discovery.
4. **Credential vault** — encrypted at rest; creds scoped per subnet/tag/device-type with match
   rules so the engine tries the right credential. Read-only service accounts recommended.
5. **Normalization & identity** — unified asset schema; dedup/merge a device seen via multiple
   paths (MAC, serial, hostname, AD GUID); relationships (VM↔hypervisor, host↔AD); change history.
6. **Storage** — PostgreSQL; normalized core tables + JSONB raw payload per collector run.
7. **Scheduler** — recurring scans per scope with interval, jitter, rate limits, concurrency caps,
   off-hours windows; internal worker pool (NATS/Redis queue only if we go distributed).
8. **Outputs** — Zabbix sync, web dashboards + REST API, CSV/JSON/webhook, optional Prometheus.
9. **Distributed collectors (phase 4)** — remote pollers per site reporting to central over mTLS.

**Zabbix integration** (confirm preferred direction — see open questions)
- **A. Host + inventory sync via Zabbix API** — auto create/update hosts, host groups, and the
  built-in *host inventory* fields from discovered assets. (Most likely what you want.)
- **B. Trapper/sender push** — send selected metrics to Zabbix trapper items.
- **C. Pull** — expose our REST API for Zabbix HTTP-agent items.
- Plan: implement **A** first, make B/C configurable.

---

## 4. Data model (sketch)
- `device` (id, identity keys, type, primary_ip, hostname, first_seen, last_seen, source)
- `interface` (device_id, mac, ip, name, speed, vlan)
- `os` (device_id, family, name, version, build, patches)
- `hardware` (device_id, vendor, model, serial, cpu, ram, disks…)
- `software` (device_id, name, version, vendor, install_date) — many per device
- `user_account` (device_id / ad, name, last_logon)
- `relationship` (parent_id, child_id, kind)  // e.g. hypervisor→vm
- `scan_run`, `scan_target`, `credential`, `schedule`
- `change_event` (device_id, field, old, new, ts) — diff timeline
- `raw_payload` (scan_run_id, device_id, collector, jsonb)

---

## 5. Security
- Web UI/API behind auth (local accounts + optional OIDC/LDAP SSO), RBAC (viewer/admin), TLS.
- Least-privilege read-only creds everywhere: SNMPv3 > v2c, SSH keys, WinRM over HTTPS.
- Credential vault encrypted at rest; secrets never logged; audit log of scans + config changes.
- Network ACLs must allow the scanner to reach: WinRM 5985/5986, SSH 22, SNMP 161, LDAP 389/636,
  and vendor API ports (vSphere 443, FortiGate/OPNsense HTTPS, Proxmox 8006).

---

## 6. Roadmap

**Phase 0 — Foundations**
Repo scaffold, config, structured logging, DB schema + migrations, asset data model,
credential vault, collector plugin interface, worker pool, basic CLI.

**Phase 1 — Discovery + core collectors**
Subnet discovery + fingerprinting; SSH and SNMP collectors first (lowest friction), then WinRM
Windows collector; normalization + dedup; persist to Postgres; CLI-driven one-off scans.

**Phase 2 — Scheduler + API + dashboards**
Recurring scans; REST API; embedded web UI (inventory browser, device detail, software rollups,
change timeline, exports, scan status); auth + RBAC.

**Phase 3 — Integrations**
AD/LDAP discovery; VMware (govmomi); Proxmox/Hyper-V (if in scope); OPNsense + FortiGate API
collectors; **Zabbix host/inventory sync**.

**Phase 4 — Scale & hardening**
Distributed remote collectors (mTLS); rate limiting/jitter; tuning for 20k hosts; SNMPv3;
audit logging; change alerting; backups.

**Phase 5 — Polish**
Reports/exports, Prometheus metrics, packaging (Docker/Helm), documentation.

---

## 7. Open questions (need your call before/while building)
1. **Windows transport**: is **WinRM enabled** fleet-wide (via GPO)? If many hosts only allow
   legacy DCOM-WMI, we add either an impacket-based sidecar or a small Windows collector node.
2. **Zabbix direction**: API host+inventory sync (A), trapper push (B), or pull (C)? Which Zabbix
   version, and do you want us to *create* hosts in Zabbix or only enrich existing ones?
3. **Hypervisors in scope**: VMware vSphere/ESXi only, or also Proxmox / Hyper-V?
4. **Dashboard frontend**: React/TS SPA (richer) vs server-rendered htmx/templ (simpler, leaner).
5. **Distributed collectors**: single central scanner for now, or multi-site remote pollers from
   the start? (Affects whether we add a queue like NATS early.)
6. **Software inventory depth on Windows**: registry Uninstall keys is standard; need MSI/Store/
   per-user apps too?
7. **Repo & naming**: project name (`motzworks`?), license, Go module path, CI choice.

## 7a. My Answers
1. Yes WinRM is installed fleetwide on the windows servers and windows pc's
2. I think Zabbix should pull. Yes create hosts if they don't exist, however this should all then be combined, i think this feature can always be added later, getting zabbix to read from a Postgress should be realtivly easy. 
3. Mainly Proxmox don't think we have anythig else but would be cool if other stuff still worked incase i eveer publish this on github
4. React/TS Dachboard
5. Single scanner, much easier to deploy aswell
6. Registry for now, we can always add to this later
7. yes motzworks for now, init a git project, don't commit yourself i will handle commiting. licence and ci i will do later once i push to github/gitlab myself
```
