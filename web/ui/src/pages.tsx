import { useEffect, useState } from "react";
import {
  api, Stats, DeviceItem, DeviceDetail, SoftwareAgg, ChangeRow, ScanRow,
  ScanTarget, Credential, Schedule,
} from "./api";
import { navigate } from "./router";

// ---- helpers ----

function useFetch<T>(path: string, extraDeps: any[] = []) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [tick, setTick] = useState(0);
  useEffect(() => {
    let live = true;
    setLoading(true);
    setError(null);
    api.get<T>(path)
      .then((d) => live && setData(d))
      .catch((e) => live && setError(e.message))
      .finally(() => live && setLoading(false));
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path, tick, ...extraDeps]);
  return { data, error, loading, reload: () => setTick((t) => t + 1) };
}

const fmtDate = (s: string | null) => (s ? new Date(s).toLocaleString() : "—");

function fmtBytes(n: number): string {
  if (!n) return "—";
  const u = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(i ? 1 : 0)} ${u[i]}`;
}

function Section({ title, children }: { title: string; children: any }) {
  return (
    <section className="card">
      <h2>{title}</h2>
      {children}
    </section>
  );
}

function ErrorBox({ error }: { error: string | null }) {
  if (!error) return null;
  return <div className="error">{error}</div>;
}

// ---- Login ----

export function Login({ onLogin }: { onLogin: () => void }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const submit = async (e: any) => {
    e.preventDefault();
    setError(null);
    try {
      await api.post("/api/login", { username, password });
      onLogin();
    } catch (err: any) {
      setError(err.message);
    }
  };
  return (
    <div className="center">
      <form className="card login" onSubmit={submit}>
        <div className="brand">motzworks</div>
        <ErrorBox error={error} />
        <label>Username<input value={username} onChange={(e) => setUsername(e.target.value)} autoFocus /></label>
        <label>Password<input type="password" value={password} onChange={(e) => setPassword(e.target.value)} /></label>
        <button type="submit">Sign in</button>
      </form>
    </div>
  );
}

// ---- Dashboard ----

export function Dashboard() {
  const { data, error } = useFetch<Stats>("/api/stats");
  return (
    <div>
      <h1>Dashboard</h1>
      <ErrorBox error={error} />
      {data && (
        <>
          <div className="stat-grid">
            <Stat label="Devices" value={data.total_devices} />
            <Stat label="Seen last 24h" value={data.seen_last_24h} />
            <Stat label="Software titles" value={data.total_software_titles} />
            <Stat label="Last scan" value={fmtDate(data.last_scan_at)} small />
          </div>
          <Section title="Devices by type">
            <TypeBars byType={data.by_type} />
          </Section>
        </>
      )}
    </div>
  );
}

function Stat({ label, value, small }: { label: string; value: any; small?: boolean }) {
  return (
    <div className="stat">
      <div className={small ? "stat-value small" : "stat-value"}>{value}</div>
      <div className="muted">{label}</div>
    </div>
  );
}

function TypeBars({ byType }: { byType: Record<string, number> }) {
  const entries = Object.entries(byType).sort((a, b) => b[1] - a[1]);
  const max = Math.max(1, ...entries.map(([, v]) => v));
  return (
    <div className="bars">
      {entries.map(([t, n]) => (
        <div className="bar-row" key={t}>
          <span className="bar-label">{t}</span>
          <span className="bar-track"><span className="bar-fill" style={{ width: `${(n / max) * 100}%` }} /></span>
          <span className="bar-value">{n}</span>
        </div>
      ))}
      {entries.length === 0 && <div className="muted">No devices yet.</div>}
    </div>
  );
}

// ---- Devices ----

export function Devices() {
  const [q, setQ] = useState("");
  const [type, setType] = useState("");
  const [offset, setOffset] = useState(0);
  const limit = 50;
  const path = `/api/devices?q=${encodeURIComponent(q)}&type=${encodeURIComponent(type)}&limit=${limit}&offset=${offset}`;
  const { data, error } = useFetch<{ items: DeviceItem[]; total: number }>(path, [q, type, offset]);
  const items = data?.items ?? [];
  const total = data?.total ?? 0;

  return (
    <div>
      <h1>Devices</h1>
      <div className="toolbar">
        <input placeholder="Search hostname / IP / serial…" value={q}
          onChange={(e) => { setQ(e.target.value); setOffset(0); }} />
        <input placeholder="type (linux, windows, switch…)" value={type}
          onChange={(e) => { setType(e.target.value); setOffset(0); }} />
        <a className="btn" href={`/api/devices.csv?q=${encodeURIComponent(q)}&type=${encodeURIComponent(type)}`}>Export CSV</a>
      </div>
      <ErrorBox error={error} />
      <table>
        <thead><tr><th>Hostname</th><th>IP</th><th>Type</th><th>OS</th><th>Source</th><th>Last seen</th></tr></thead>
        <tbody>
          {items.map((d) => (
            <tr key={d.id} className="clickable" onClick={() => navigate(`/devices/${d.id}`)}>
              <td>{d.hostname || "—"}</td>
              <td>{d.primary_ip || "—"}</td>
              <td><span className="tag">{d.type}</span></td>
              <td>{d.os_name || "—"}</td>
              <td>{d.source}</td>
              <td>{fmtDate(d.last_seen)}</td>
            </tr>
          ))}
          {items.length === 0 && <tr><td colSpan={6} className="muted">No devices.</td></tr>}
        </tbody>
      </table>
      <Pager total={total} limit={limit} offset={offset} setOffset={setOffset} />
    </div>
  );
}

function Pager({ total, limit, offset, setOffset }:
  { total: number; limit: number; offset: number; setOffset: (n: number) => void }) {
  const from = total === 0 ? 0 : offset + 1;
  const to = Math.min(offset + limit, total);
  return (
    <div className="pager">
      <button disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - limit))}>Prev</button>
      <span className="muted">{from}–{to} of {total}</span>
      <button disabled={to >= total} onClick={() => setOffset(offset + limit)}>Next</button>
    </div>
  );
}

// ---- Device detail ----

export function DeviceDetailPage({ id }: { id: string }) {
  const { data: d, error } = useFetch<DeviceDetail>(`/api/devices/${id}`);
  const { data: changes } = useFetch<{ items: ChangeRow[] }>(`/api/changes?device_id=${id}&limit=50`);
  if (error) return <ErrorBox error={error} />;
  if (!d) return <div className="muted">Loading…</div>;
  return (
    <div>
      <a href="#/devices" className="back">← Devices</a>
      <h1>{d.hostname || d.primary_ip}</h1>
      <div className="detail-grid">
        <Section title="Overview">
          <KV k="Type" v={d.type} />
          <KV k="IP" v={d.primary_ip} />
          <KV k="Serial" v={d.serial} />
          <KV k="Source" v={d.source} />
          <KV k="First seen" v={fmtDate(d.first_seen)} />
          <KV k="Last seen" v={fmtDate(d.last_seen)} />
        </Section>
        {d.os && (
          <Section title="Operating system">
            <KV k="Name" v={d.os.name} />
            <KV k="Version" v={d.os.version} />
            <KV k="Build" v={d.os.build} />
            <KV k="Arch" v={d.os.arch} />
          </Section>
        )}
        {d.hardware && (
          <Section title="Hardware">
            <KV k="Vendor" v={d.hardware.vendor} />
            <KV k="Model" v={d.hardware.model} />
            <KV k="CPU" v={d.hardware.cpu} />
            <KV k="Cores" v={d.hardware.cpu_cores || "—"} />
            <KV k="RAM" v={fmtBytes(d.hardware.ram_bytes)} />
          </Section>
        )}
      </div>

      <Section title={`Interfaces (${d.interfaces.length})`}>
        <table>
          <thead><tr><th>Name</th><th>MAC</th><th>IP</th><th>Speed</th></tr></thead>
          <tbody>
            {d.interfaces.map((i, idx) => (
              <tr key={idx}><td>{i.name}</td><td>{i.mac || "—"}</td><td>{i.ip || "—"}</td>
                <td>{i.speed_mbps ? `${i.speed_mbps} Mbps` : "—"}</td></tr>
            ))}
            {d.interfaces.length === 0 && <tr><td colSpan={4} className="muted">None</td></tr>}
          </tbody>
        </table>
      </Section>

      <Section title={`Software (${d.software.length})`}>
        <table>
          <thead><tr><th>Name</th><th>Version</th><th>Vendor</th></tr></thead>
          <tbody>
            {d.software.map((s, idx) => (
              <tr key={idx}><td>{s.name}</td><td>{s.version || "—"}</td><td>{s.vendor || "—"}</td></tr>
            ))}
            {d.software.length === 0 && <tr><td colSpan={3} className="muted">None</td></tr>}
          </tbody>
        </table>
      </Section>

      <Section title={`Users (${d.users.length})`}>
        <table>
          <thead><tr><th>Username</th><th>Full name</th><th>Local</th></tr></thead>
          <tbody>
            {d.users.map((u, idx) => (
              <tr key={idx}><td>{u.username}</td><td>{u.full_name || "—"}</td><td>{u.is_local ? "yes" : "no"}</td></tr>
            ))}
            {d.users.length === 0 && <tr><td colSpan={3} className="muted">None</td></tr>}
          </tbody>
        </table>
      </Section>

      <Section title="Recent changes">
        <ChangeTable rows={changes?.items ?? []} showHost={false} />
      </Section>
    </div>
  );
}

function KV({ k, v }: { k: string; v: any }) {
  return <div className="kv"><span className="kv-k">{k}</span><span className="kv-v">{v || "—"}</span></div>;
}

// ---- Software ----

export function Software() {
  const [q, setQ] = useState("");
  const { data, error } = useFetch<{ items: SoftwareAgg[] }>(`/api/software?q=${encodeURIComponent(q)}&limit=300`, [q]);
  const items = data?.items ?? [];
  return (
    <div>
      <h1>Software</h1>
      <div className="toolbar">
        <input placeholder="Search software…" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      <ErrorBox error={error} />
      <table>
        <thead><tr><th>Name</th><th>Version</th><th>Devices</th></tr></thead>
        <tbody>
          {items.map((s, idx) => (
            <tr key={idx}><td>{s.name}</td><td>{s.version || "—"}</td><td>{s.device_count}</td></tr>
          ))}
          {items.length === 0 && <tr><td colSpan={3} className="muted">No software.</td></tr>}
        </tbody>
      </table>
    </div>
  );
}

// ---- Changes ----

export function Changes() {
  const { data, error } = useFetch<{ items: ChangeRow[] }>("/api/changes?limit=200");
  return (
    <div>
      <h1>Changes</h1>
      <ErrorBox error={error} />
      <ChangeTable rows={data?.items ?? []} showHost />
    </div>
  );
}

function ChangeTable({ rows, showHost }: { rows: ChangeRow[]; showHost: boolean }) {
  return (
    <table>
      <thead><tr>{showHost && <th>Device</th>}<th>Field</th><th>Old</th><th>New</th><th>When</th></tr></thead>
      <tbody>
        {rows.map((c, idx) => (
          <tr key={idx}>
            {showHost && <td className="clickable" onClick={() => navigate(`/devices/${c.device_id}`)}>{c.hostname}</td>}
            <td>{c.field}</td><td className="muted">{c.old_value || "—"}</td><td>{c.new_value || "—"}</td>
            <td>{fmtDate(c.ts)}</td>
          </tr>
        ))}
        {rows.length === 0 && <tr><td colSpan={showHost ? 5 : 4} className="muted">No changes.</td></tr>}
      </tbody>
    </table>
  );
}

// ---- Scans ----

export function Scans() {
  const { data, error } = useFetch<{ items: ScanRow[] }>("/api/scans?limit=50");
  return (
    <div>
      <h1>Scans</h1>
      <ErrorBox error={error} />
      <table>
        <thead><tr><th>Started</th><th>Finished</th><th>Status</th><th>Hosts</th><th>Error</th></tr></thead>
        <tbody>
          {(data?.items ?? []).map((s) => (
            <tr key={s.id}>
              <td>{fmtDate(s.started_at)}</td><td>{fmtDate(s.finished_at)}</td>
              <td><span className={`tag status-${s.status}`}>{s.status}</span></td>
              <td>{s.hosts_found}</td><td className="muted">{s.error || "—"}</td>
            </tr>
          ))}
          {(data?.items ?? []).length === 0 && <tr><td colSpan={5} className="muted">No scans.</td></tr>}
        </tbody>
      </table>
    </div>
  );
}

// ---- Admin ----

export function Admin() {
  const targets = useFetch<{ items: ScanTarget[] }>("/api/targets");
  const creds = useFetch<{ items: Credential[] }>("/api/credentials");
  const schedules = useFetch<{ items: Schedule[] }>("/api/schedules");
  const [msg, setMsg] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const flash = (m: string) => { setMsg(m); setErr(null); setTimeout(() => setMsg(null), 3000); };
  const fail = (e: any) => { setErr(e.message); setMsg(null); };

  // forms state
  const [adhoc, setAdhoc] = useState("");
  const [tName, setTName] = useState("");
  const [tCidrs, setTCidrs] = useState("");
  const [cName, setCName] = useState("");
  const [cKind, setCKind] = useState("ssh-password");
  const [cUser, setCUser] = useState("");
  const [cSecret, setCSecret] = useState("");
  const [sTarget, setSTarget] = useState("");
  const [sInterval, setSInterval] = useState("3600");

  const triggerScan = async () => {
    try { await api.post("/api/scans", { targets: adhoc.split(",").map((s) => s.trim()).filter(Boolean) }); flash("Scan started"); }
    catch (e) { fail(e); }
  };
  const createTarget = async () => {
    try { await api.post("/api/targets", { name: tName, cidrs: tCidrs.split(",").map((s) => s.trim()).filter(Boolean) }); setTName(""); setTCidrs(""); flash("Target created"); targets.reload(); }
    catch (e) { fail(e); }
  };
  const delTarget = async (id: string) => { try { await api.del(`/api/targets/${id}`); targets.reload(); } catch (e) { fail(e); } };
  const createCred = async () => {
    try { await api.post("/api/credentials", { name: cName, kind: cKind, username: cUser, secret: cSecret }); setCName(""); setCUser(""); setCSecret(""); flash("Credential saved (sealed)"); creds.reload(); }
    catch (e) { fail(e); }
  };
  const delCred = async (id: string) => { try { await api.del(`/api/credentials/${id}`); creds.reload(); } catch (e) { fail(e); } };
  const createSchedule = async () => {
    try { await api.post("/api/schedules", { scan_target_id: sTarget, interval_secs: parseInt(sInterval, 10) }); flash("Schedule created"); schedules.reload(); }
    catch (e) { fail(e); }
  };

  return (
    <div>
      <h1>Admin</h1>
      {msg && <div className="ok">{msg}</div>}
      <ErrorBox error={err} />

      <Section title="Run ad-hoc scan">
        <div className="toolbar">
          <input placeholder="10.0.0.0/24, 10.0.1.5" value={adhoc} onChange={(e) => setAdhoc(e.target.value)} style={{ flex: 1 }} />
          <button onClick={triggerScan}>Scan now</button>
        </div>
        <div className="muted small">Uses all stored credentials.</div>
      </Section>

      <Section title="Scan targets">
        <table>
          <thead><tr><th>Name</th><th>CIDRs</th><th>Enabled</th><th></th></tr></thead>
          <tbody>
            {(targets.data?.items ?? []).map((t) => (
              <tr key={t.id}><td>{t.name}</td><td>{t.cidrs.join(", ")}</td><td>{t.enabled ? "yes" : "no"}</td>
                <td><button className="danger" onClick={() => delTarget(t.id)}>Delete</button></td></tr>
            ))}
          </tbody>
        </table>
        <div className="toolbar">
          <input placeholder="name" value={tName} onChange={(e) => setTName(e.target.value)} />
          <input placeholder="cidrs (comma-separated)" value={tCidrs} onChange={(e) => setTCidrs(e.target.value)} style={{ flex: 1 }} />
          <button onClick={createTarget}>Add target</button>
        </div>
      </Section>

      <Section title="Credentials">
        <table>
          <thead><tr><th>Name</th><th>Kind</th><th>Username</th><th></th></tr></thead>
          <tbody>
            {(creds.data?.items ?? []).map((c) => (
              <tr key={c.id}><td>{c.name}</td><td>{c.kind}</td><td>{c.username || "—"}</td>
                <td><button className="danger" onClick={() => delCred(c.id)}>Delete</button></td></tr>
            ))}
          </tbody>
        </table>
        <div className="toolbar">
          <input placeholder="name" value={cName} onChange={(e) => setCName(e.target.value)} />
          <select value={cKind} onChange={(e) => setCKind(e.target.value)}>
            <option value="ssh-password">ssh-password</option>
            <option value="ssh-key">ssh-key</option>
            <option value="winrm">winrm</option>
            <option value="snmp-v2c">snmp-v2c</option>
          </select>
          <input placeholder="username" value={cUser} onChange={(e) => setCUser(e.target.value)} />
          <input placeholder="secret / community / key" type="password" value={cSecret} onChange={(e) => setCSecret(e.target.value)} style={{ flex: 1 }} />
          <button onClick={createCred}>Add credential</button>
        </div>
      </Section>

      <Section title="Schedules">
        <table>
          <thead><tr><th>Target</th><th>Interval</th><th>Enabled</th><th>Next run</th></tr></thead>
          <tbody>
            {(schedules.data?.items ?? []).map((s) => (
              <tr key={s.id}><td>{targetName(targets.data?.items, s.scan_target_id)}</td>
                <td>{s.interval_secs}s</td><td>{s.enabled ? "yes" : "no"}</td><td>{fmtDate(s.next_run_at)}</td></tr>
            ))}
          </tbody>
        </table>
        <div className="toolbar">
          <select value={sTarget} onChange={(e) => setSTarget(e.target.value)}>
            <option value="">select target…</option>
            {(targets.data?.items ?? []).map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
          </select>
          <input placeholder="interval (seconds)" value={sInterval} onChange={(e) => setSInterval(e.target.value)} />
          <button onClick={createSchedule}>Add schedule</button>
        </div>
      </Section>
    </div>
  );
}

function targetName(targets: ScanTarget[] | undefined, id: string): string {
  return targets?.find((t) => t.id === id)?.name ?? id.slice(0, 8);
}
