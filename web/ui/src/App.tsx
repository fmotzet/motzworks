import { useEffect, useState } from "react";
import { api, Me } from "./api";
import { useHashRoute, navigate, deviceIdFromRoute } from "./router";
import {
  Login, Dashboard, Devices, DeviceDetailPage, Software, Changes, Scans, Admin,
} from "./pages";

const NAV = [
  { path: "/", label: "Dashboard" },
  { path: "/devices", label: "Devices" },
  { path: "/software", label: "Software" },
  { path: "/changes", label: "Changes" },
  { path: "/scans", label: "Scans" },
  { path: "/admin", label: "Admin", adminOnly: true },
];

export function App() {
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);
  const route = useHashRoute();

  const refreshMe = () =>
    api.get<Me>("/api/me").then(setMe).catch(() => setMe(null)).finally(() => setLoading(false));

  useEffect(() => {
    refreshMe();
    const onUnauth = () => setMe(null);
    window.addEventListener("unauthorized", onUnauth);
    return () => window.removeEventListener("unauthorized", onUnauth);
  }, []);

  if (loading) return <div className="center muted">Loading…</div>;
  if (!me) return <Login onLogin={refreshMe} />;

  const logout = async () => {
    await api.post("/api/logout");
    setMe(null);
    navigate("/");
  };

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="brand">motzworks</div>
        <nav>
          {NAV.filter((n) => !n.adminOnly || me.role === "admin").map((n) => (
            <a key={n.path} className={isActive(route, n.path) ? "active" : ""} href={"#" + n.path}>
              {n.label}
            </a>
          ))}
        </nav>
        <div className="user">
          <div>{me.username}</div>
          <div className="muted small">{me.role}</div>
          <button onClick={logout}>Log out</button>
        </div>
      </aside>
      <main className="content">
        <Routed route={route} role={me.role} />
      </main>
    </div>
  );
}

function isActive(route: string, path: string): boolean {
  if (path === "/") return route === "/";
  return route === path || route.startsWith(path + "/");
}

function Routed({ route, role }: { route: string; role: string }) {
  const devId = deviceIdFromRoute(route);
  if (devId) return <DeviceDetailPage id={devId} />;
  switch (route) {
    case "/": return <Dashboard />;
    case "/devices": return <Devices />;
    case "/software": return <Software />;
    case "/changes": return <Changes />;
    case "/scans": return <Scans />;
    case "/admin": return role === "admin" ? <Admin /> : <div className="muted">Forbidden</div>;
    default: return <div className="muted">Not found</div>;
  }
}
