package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// ScanTarget is a named set of CIDRs/IPs to scan.
type ScanTarget struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	CIDRs   []string `json:"cidrs"`
	Enabled bool     `json:"enabled"`
}

// CreateScanTarget inserts a scan target and returns its id.
func (s *Store) CreateScanTarget(ctx context.Context, name string, cidrs []string, enabled bool) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO scan_target (name, cidrs, enabled) VALUES ($1,$2,$3) RETURNING id`,
		name, cidrs, enabled,
	).Scan(&id)
	return id, err
}

// ListScanTargets returns all scan targets.
func (s *Store) ListScanTargets(ctx context.Context) ([]ScanTarget, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, cidrs, enabled FROM scan_target ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScanTarget
	for rows.Next() {
		var t ScanTarget
		if err := rows.Scan(&t.ID, &t.Name, &t.CIDRs, &t.Enabled); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetScanTarget loads a single target, or (nil, nil) if not found.
func (s *Store) GetScanTarget(ctx context.Context, id string) (*ScanTarget, error) {
	var t ScanTarget
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, cidrs, enabled FROM scan_target WHERE id = $1`, id,
	).Scan(&t.ID, &t.Name, &t.CIDRs, &t.Enabled)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// DeleteScanTarget removes a target.
func (s *Store) DeleteScanTarget(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM scan_target WHERE id = $1`, id)
	return err
}

// StoredCredential is a credential with its secret still sealed.
type StoredCredential struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Kind         string            `json:"kind"`
	Username     string            `json:"username"`
	SecretSealed string            `json:"-"`
	Extra        map[string]string `json:"extra"`
}

// CreateCredential stores a credential with its secret already sealed.
func (s *Store) CreateCredential(ctx context.Context, name, kind, username, sealed string, extra map[string]string) (string, error) {
	if extra == nil {
		extra = map[string]string{}
	}
	extraJSON, err := json.Marshal(extra)
	if err != nil {
		return "", err
	}
	var id string
	err = s.pool.QueryRow(ctx, `
		INSERT INTO credential (name, kind, username, secret_sealed, extra)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		name, kind, username, sealed, extraJSON,
	).Scan(&id)
	return id, err
}

// DeleteCredential removes a credential.
func (s *Store) DeleteCredential(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM credential WHERE id = $1`, id)
	return err
}

// ListCredentials returns all stored credentials (secrets remain sealed).
func (s *Store) ListCredentials(ctx context.Context) ([]StoredCredential, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, kind, COALESCE(username,''), COALESCE(secret_sealed,''), extra FROM credential ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredCredential
	for rows.Next() {
		var c StoredCredential
		var extraRaw []byte
		if err := rows.Scan(&c.ID, &c.Name, &c.Kind, &c.Username, &c.SecretSealed, &extraRaw); err != nil {
			return nil, err
		}
		c.Extra = map[string]string{}
		if len(extraRaw) > 0 {
			_ = json.Unmarshal(extraRaw, &c.Extra)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Schedule drives recurring scans of a target.
type Schedule struct {
	ID           string     `json:"id"`
	ScanTargetID string     `json:"scan_target_id"`
	IntervalSecs int        `json:"interval_secs"`
	Enabled      bool       `json:"enabled"`
	NextRunAt    *time.Time `json:"next_run_at"`
}

// CreateSchedule inserts a schedule due immediately (next_run_at = now).
func (s *Store) CreateSchedule(ctx context.Context, scanTargetID string, intervalSecs int, enabled bool) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO schedule (scan_target_id, interval_secs, enabled, next_run_at)
		VALUES ($1,$2,$3, now()) RETURNING id`,
		scanTargetID, intervalSecs, enabled,
	).Scan(&id)
	return id, err
}

// ListSchedules returns all schedules.
func (s *Store) ListSchedules(ctx context.Context) ([]Schedule, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, scan_target_id, interval_secs, enabled, next_run_at FROM schedule ORDER BY next_run_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Schedule
	for rows.Next() {
		var sc Schedule
		if err := rows.Scan(&sc.ID, &sc.ScanTargetID, &sc.IntervalSecs, &sc.Enabled, &sc.NextRunAt); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// DueSchedule is a schedule whose target is ready to scan.
type DueSchedule struct {
	ScheduleID   string
	ScanTargetID string
	IntervalSecs int
	CIDRs        []string
}

// DueSchedules returns enabled schedules whose next_run_at has passed, joined
// with their (enabled) target's CIDRs.
func (s *Store) DueSchedules(ctx context.Context) ([]DueSchedule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT sc.id, t.id, sc.interval_secs, t.cidrs
		FROM schedule sc
		JOIN scan_target t ON t.id = sc.scan_target_id
		WHERE sc.enabled AND t.enabled
		  AND (sc.next_run_at IS NULL OR sc.next_run_at <= now())`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DueSchedule
	for rows.Next() {
		var d DueSchedule
		if err := rows.Scan(&d.ScheduleID, &d.ScanTargetID, &d.IntervalSecs, &d.CIDRs); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// AdvanceSchedule sets next_run_at to now + interval.
func (s *Store) AdvanceSchedule(ctx context.Context, scheduleID string, intervalSecs int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE schedule SET next_run_at = now() + make_interval(secs => $2) WHERE id = $1`,
		scheduleID, intervalSecs)
	return err
}
