// Package api exposes the REST API for the dashboard: read endpoints for the
// inventory (viewer role) and management endpoints for scans, targets,
// credentials and schedules (admin role). Auth is via an HMAC-signed session
// cookie issued at login.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/stock3/motzworks/internal/auth"
	"github.com/stock3/motzworks/internal/scan"
	"github.com/stock3/motzworks/internal/store"
	"github.com/stock3/motzworks/internal/vault"
)

const sessionCookie = "mw_session"

// Server holds API dependencies.
type Server struct {
	store        *store.Store
	engine       *scan.Engine
	vault        *vault.Vault
	log          *slog.Logger
	secret       []byte
	ttl          time.Duration
	concurrency  int
	secureCookie bool
	static       http.Handler
}

// Options configure the API server.
type Options struct {
	Secret       []byte
	SessionTTL   time.Duration
	Concurrency  int
	SecureCookie bool
	Static       http.Handler // SPA handler for non-API routes
}

// New builds an API server.
func New(st *store.Store, eng *scan.Engine, v *vault.Vault, log *slog.Logger, opts Options) *Server {
	if opts.SessionTTL <= 0 {
		opts.SessionTTL = 12 * time.Hour
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 32
	}
	return &Server{
		store: st, engine: eng, vault: v, log: log,
		secret: opts.Secret, ttl: opts.SessionTTL, concurrency: opts.Concurrency,
		secureCookie: opts.SecureCookie, static: opts.Static,
	}
}

// Handler returns the root HTTP handler with all routes and middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public.
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /metrics", s.handleMetrics)

	// Authenticated (any role).
	mux.Handle("GET /api/me", s.auth(s.handleMe))
	mux.Handle("GET /api/stats", s.auth(s.handleStats))
	mux.Handle("GET /api/devices", s.auth(s.handleDevices))
	mux.Handle("GET /api/devices/{id}", s.auth(s.handleDevice))
	mux.Handle("GET /api/devices.csv", s.auth(s.handleDevicesCSV))
	mux.Handle("GET /api/software", s.auth(s.handleSoftware))
	mux.Handle("GET /api/software/devices", s.auth(s.handleSoftwareDevices))
	mux.Handle("GET /api/changes", s.auth(s.handleChanges))
	mux.Handle("GET /api/scans", s.auth(s.handleScans))
	mux.Handle("GET /api/scans/{id}", s.auth(s.handleScanDetail))
	mux.Handle("GET /api/targets", s.auth(s.handleListTargets))
	mux.Handle("GET /api/schedules", s.auth(s.handleListSchedules))

	// Admin only.
	mux.Handle("POST /api/scans", s.admin(s.handleTriggerScan))
	mux.Handle("POST /api/targets", s.admin(s.handleCreateTarget))
	mux.Handle("DELETE /api/targets/{id}", s.admin(s.handleDeleteTarget))
	mux.Handle("GET /api/credentials", s.admin(s.handleListCredentials))
	mux.Handle("POST /api/credentials", s.admin(s.handleCreateCredential))
	mux.Handle("DELETE /api/credentials/{id}", s.admin(s.handleDeleteCredential))
	mux.Handle("POST /api/schedules", s.admin(s.handleCreateSchedule))
	mux.Handle("GET /api/audit", s.admin(s.handleAudit))

	// Unknown API routes → JSON 404 (more specific than the SPA catch-all).
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})

	// SPA for everything else.
	if s.static != nil {
		mux.Handle("/", s.static)
	}

	return s.recoverMW(s.logMW(mux))
}

// ---- middleware ----

type ctxKey int

const claimsKey ctxKey = iota

func (s *Server) auth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := s.claimsFromRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), claimsKey, c)))
	})
}

func (s *Server) admin(next http.HandlerFunc) http.Handler {
	return s.auth(func(w http.ResponseWriter, r *http.Request) {
		if claimsOf(r).Role != auth.RoleAdmin {
			writeError(w, http.StatusForbidden, "admin role required")
			return
		}
		next(w, r)
	})
}

func (s *Server) logMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path,
			"status", sw.status, "dur", time.Since(start).String())
	})
}

func (s *Server) recoverMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic", "path", r.URL.Path, "recover", rec)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// ---- auth handlers ----

func (s *Server) claimsFromRequest(r *http.Request) (auth.Claims, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return auth.Claims{}, err
	}
	return auth.VerifyToken(s.secret, c.Value)
}

func claimsOf(r *http.Request) auth.Claims {
	c, _ := r.Context().Value(claimsKey).(auth.Claims)
	return c
}

// clientIP returns the best-effort client address (honoring X-Forwarded-For
// when behind a reverse proxy).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// audit records an action by the authenticated actor (best-effort; never blocks
// the request on failure).
func (s *Server) audit(r *http.Request, action, target string, detail map[string]any) {
	if err := s.store.InsertAudit(r.Context(), claimsOf(r).Username, action, target, clientIP(r), detail); err != nil {
		s.log.Warn("audit insert failed", "action", action, "err", err)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	u, err := s.store.GetUserByUsername(r.Context(), req.Username)
	if err != nil || !auth.CheckPassword(u.PasswordHash, req.Password) {
		_ = s.store.InsertAudit(r.Context(), req.Username, "login_failed", "", clientIP(r), nil)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	exp := time.Now().Add(s.ttl)
	token, err := auth.IssueToken(s.secret, auth.Claims{
		UserID: u.ID, Username: u.Username, Role: u.Role, Expires: exp.Unix(),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/",
		HttpOnly: true, Secure: s.secureCookie, SameSite: http.SameSiteLaxMode,
		Expires: exp,
	})
	_ = s.store.TouchLogin(r.Context(), u.ID)
	_ = s.store.InsertAudit(r.Context(), u.Username, "login", "", clientIP(r), nil)
	writeJSON(w, http.StatusOK, map[string]any{"username": u.Username, "role": u.Role})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/",
		HttpOnly: true, Secure: s.secureCookie, SameSite: http.SameSiteLaxMode,
		MaxAge: -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	c := claimsOf(r)
	writeJSON(w, http.StatusOK, map[string]any{"username": c.Username, "role": c.Role})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---- JSON helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}
