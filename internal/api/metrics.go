package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/stock3/motzworks/internal/version"
)

// handleMetrics exposes inventory metrics in Prometheus text exposition format.
// It is unauthenticated (the standard Prometheus convention) and reports only
// aggregate counts — no device specifics or secrets.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Stats(r.Context())
	if err != nil {
		http.Error(w, "# stats error\n", http.StatusInternalServerError)
		return
	}

	var b strings.Builder
	metric := func(name, typ, help string) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
	}

	metric("motzworks_build_info", "gauge", "Build version (always 1).")
	fmt.Fprintf(&b, "motzworks_build_info{version=%q} 1\n", version.Version)

	metric("motzworks_devices_total", "gauge", "Total inventoried devices.")
	fmt.Fprintf(&b, "motzworks_devices_total %d\n", st.TotalDevices)

	metric("motzworks_devices_seen_24h", "gauge", "Devices seen in the last 24h.")
	fmt.Fprintf(&b, "motzworks_devices_seen_24h %d\n", st.SeenLast24h)

	metric("motzworks_software_titles_total", "gauge", "Distinct software titles.")
	fmt.Fprintf(&b, "motzworks_software_titles_total %d\n", st.TotalSoftware)

	metric("motzworks_devices_by_type", "gauge", "Devices by type.")
	for t, n := range st.ByType {
		fmt.Fprintf(&b, "motzworks_devices_by_type{type=%q} %d\n", t, n)
	}

	metric("motzworks_last_scan_timestamp_seconds", "gauge", "Unix time of the most recent scan start (0 if none).")
	var lastScan int64
	if st.LastScanAt != nil {
		lastScan = st.LastScanAt.Unix()
	}
	fmt.Fprintf(&b, "motzworks_last_scan_timestamp_seconds %d\n", lastScan)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}
