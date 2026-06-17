import { useEffect, useState } from "react";

// Minimal hash-based router — no external dependency.

export function useHashRoute(): string {
  const [route, setRoute] = useState(() => window.location.hash.slice(1) || "/");
  useEffect(() => {
    const onChange = () => setRoute(window.location.hash.slice(1) || "/");
    window.addEventListener("hashchange", onChange);
    return () => window.removeEventListener("hashchange", onChange);
  }, []);
  return route;
}

export function navigate(to: string) {
  window.location.hash = to;
}

// matchRoute returns the path param for patterns like "/devices/:id".
export function deviceIdFromRoute(route: string): string | null {
  const m = route.match(/^\/devices\/(.+)$/);
  return m ? m[1] : null;
}

export function scanIdFromRoute(route: string): string | null {
  const m = route.match(/^\/scans\/(.+)$/);
  return m ? m[1] : null;
}
