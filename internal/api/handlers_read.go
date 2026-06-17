package api

import (
	"encoding/csv"
	"net/http"
	"strconv"

	"github.com/stock3/motzworks/internal/store"
)

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Stats(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	f := store.DeviceFilter{
		Query:  r.URL.Query().Get("q"),
		Type:   r.URL.Query().Get("type"),
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}
	items, total, err := s.store.ListDevices(r.Context(), f)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "limit": f.Limit, "offset": f.Offset,
	})
}

func (s *Server) handleDevice(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.GetDevice(r.Context(), r.PathValue("id"))
	if err != nil {
		s.serverError(w, err)
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDevicesCSV(w http.ResponseWriter, r *http.Request) {
	f := store.DeviceFilter{
		Query: r.URL.Query().Get("q"),
		Type:  r.URL.Query().Get("type"),
		Limit: 500,
	}
	items, _, err := s.store.ListDevices(r.Context(), f)
	if err != nil {
		s.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="devices.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "type", "hostname", "primary_ip", "os", "source", "last_seen"})
	for _, d := range items {
		_ = cw.Write([]string{d.ID, d.Type, d.Hostname, d.PrimaryIP, d.OSName, d.Source, d.LastSeen.Format("2006-01-02 15:04:05")})
	}
	cw.Flush()
}

func (s *Server) handleSoftware(w http.ResponseWriter, r *http.Request) {
	rollup, err := s.store.SoftwareRollup(r.Context(), r.URL.Query().Get("q"), queryInt(r, "limit", 200), queryInt(r, "offset", 0))
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rollup})
}

func (s *Server) handleChanges(w http.ResponseWriter, r *http.Request) {
	changes, err := s.store.ListChanges(r.Context(), r.URL.Query().Get("device_id"), queryInt(r, "limit", 100))
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": changes})
}

func (s *Server) handleScans(w http.ResponseWriter, r *http.Request) {
	scans, err := s.store.ListScans(r.Context(), queryInt(r, "limit", 50))
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": scans})
}

func (s *Server) handleScanDetail(w http.ResponseWriter, r *http.Request) {
	run, err := s.store.GetScanRun(r.Context(), r.PathValue("id"))
	if err != nil {
		s.serverError(w, err)
		return
	}
	if run == nil {
		writeError(w, http.StatusNotFound, "scan not found")
		return
	}
	events, err := s.store.ListScanEvents(r.Context(), run.ID, queryInt(r, "limit", 1000))
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scan": run, "events": events})
}

// serverError logs and returns a 500.
func (s *Server) serverError(w http.ResponseWriter, err error) {
	s.log.Error("api error", "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
