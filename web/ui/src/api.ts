// Thin fetch wrapper. Cookies carry the session, so credentials are always
// included. A 401 broadcasts an "unauthorized" event so the app can drop to the
// login screen.

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    credentials: "include",
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (res.status === 401) {
    window.dispatchEvent(new Event("unauthorized"));
    throw new Error("unauthorized");
  }
  const ct = res.headers.get("content-type") || "";
  const data = ct.includes("json") ? await res.json() : await res.text();
  if (!res.ok) {
    const msg = data && (data as any).error ? (data as any).error : res.statusText;
    throw new Error(msg);
  }
  return data as T;
}

export const api = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T>(path: string, body?: unknown) => request<T>("POST", path, body ?? {}),
  del: <T>(path: string) => request<T>("DELETE", path),
};

export interface Me {
  username: string;
  role: string;
}

export interface Stats {
  total_devices: number;
  seen_last_24h: number;
  by_type: Record<string, number>;
  last_scan_at: string | null;
  total_software_titles: number;
}

export interface DeviceItem {
  id: string;
  type: string;
  hostname: string;
  primary_ip: string;
  os_name: string;
  source: string;
  last_seen: string;
}

export interface DeviceDetail extends DeviceItem {
  serial: string;
  first_seen: string;
  os: { family: string; name: string; version: string; build: string; arch: string } | null;
  hardware: {
    vendor: string; model: string; serial: string;
    cpu: string; cpu_cores: number; ram_bytes: number;
  } | null;
  interfaces: { name: string; mac: string; ip: string; speed_mbps: number }[];
  software: { name: string; version: string; vendor: string }[];
  users: { username: string; full_name: string; last_logon: string | null; is_local: boolean }[];
}

export interface SoftwareAgg { name: string; version: string; device_count: number }
export interface ChangeRow { device_id: string; hostname: string; field: string; old_value: string; new_value: string; ts: string }
export interface ScanRow { id: string; started_at: string; finished_at: string | null; status: string; hosts_found: number; error: string }
export interface ScanTarget { id: string; name: string; cidrs: string[]; enabled: boolean }
export interface Credential { id: string; name: string; kind: string; username: string; extra: Record<string, string> }
export interface Schedule { id: string; scan_target_id: string; interval_secs: number; enabled: boolean; next_run_at: string | null }
