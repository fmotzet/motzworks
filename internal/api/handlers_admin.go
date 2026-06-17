package api

import (
	"context"
	"net/http"

	"github.com/stock3/motzworks/internal/scan"
)

// handleTriggerScan kicks off an ad-hoc scan. Body: {"targets":["10.0.0.0/24"]}
// or {"target_id":"<uuid>"}. The scan runs in the background using all stored
// credentials; clients poll GET /api/scans for progress.
func (s *Server) handleTriggerScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Targets  []string `json:"targets"`
		TargetID string   `json:"target_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	specs := req.Targets
	if req.TargetID != "" {
		t, err := s.store.GetScanTarget(r.Context(), req.TargetID)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if t == nil {
			writeError(w, http.StatusNotFound, "scan target not found")
			return
		}
		specs = t.CIDRs
	}
	if len(specs) == 0 {
		writeError(w, http.StatusBadRequest, "no targets specified")
		return
	}

	stored, err := s.store.ListCredentials(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	creds := scan.DecryptCredentials(stored, s.vault)

	go func() {
		ctx := context.Background()
		opts := scan.Options{
			Specs:          specs,
			Credentials:    creds,
			CollectWorkers: s.concurrency,
			Probe:          scan.SNMPProbe(creds, s.concurrency, s.log),
		}
		if _, err := s.engine.Run(ctx, opts); err != nil {
			s.log.Error("ad-hoc scan failed", "err", err)
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "scan started"})
}

// ---- scan targets ----

func (s *Server) handleListTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := s.store.ListScanTargets(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": targets})
}

func (s *Server) handleCreateTarget(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string   `json:"name"`
		CIDRs   []string `json:"cidrs"`
		Enabled *bool    `json:"enabled"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Name == "" || len(req.CIDRs) == 0 {
		writeError(w, http.StatusBadRequest, "name and cidrs are required")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	id, err := s.store.CreateScanTarget(r.Context(), req.Name, req.CIDRs, enabled)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) handleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteScanTarget(r.Context(), r.PathValue("id")); err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---- credentials ----

func (s *Server) handleListCredentials(w http.ResponseWriter, r *http.Request) {
	creds, err := s.store.ListCredentials(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": creds})
}

func (s *Server) handleCreateCredential(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string            `json:"name"`
		Kind     string            `json:"kind"`
		Username string            `json:"username"`
		Secret   string            `json:"secret"`
		Extra    map[string]string `json:"extra"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Name == "" || req.Kind == "" {
		writeError(w, http.StatusBadRequest, "name and kind are required")
		return
	}
	sealed := ""
	if req.Secret != "" {
		var err error
		sealed, err = s.vault.Seal([]byte(req.Secret))
		if err != nil {
			s.serverError(w, err)
			return
		}
	}
	id, err := s.store.CreateCredential(r.Context(), req.Name, req.Kind, req.Username, sealed, req.Extra)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) handleDeleteCredential(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteCredential(r.Context(), r.PathValue("id")); err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---- schedules ----

func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	schedules, err := s.store.ListSchedules(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": schedules})
}

func (s *Server) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ScanTargetID string `json:"scan_target_id"`
		IntervalSecs int    `json:"interval_secs"`
		Enabled      *bool  `json:"enabled"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.ScanTargetID == "" || req.IntervalSecs < 60 {
		writeError(w, http.StatusBadRequest, "scan_target_id and interval_secs (>=60) are required")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	id, err := s.store.CreateSchedule(r.Context(), req.ScanTargetID, req.IntervalSecs, enabled)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}
