package store

import (
	"context"
	"encoding/json"
	"time"
)

// AuditEntry is one audit-log record.
type AuditEntry struct {
	ID     string         `json:"id"`
	TS     time.Time      `json:"ts"`
	Actor  string         `json:"actor"`
	Action string         `json:"action"`
	Target string         `json:"target"`
	IP     string         `json:"ip"`
	Detail map[string]any `json:"detail"`
}

// InsertAudit appends an audit record. detail may be nil.
func (s *Store) InsertAudit(ctx context.Context, actor, action, target, ip string, detail map[string]any) error {
	if detail == nil {
		detail = map[string]any{}
	}
	d, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO audit_log (actor, action, target, ip, detail)
		VALUES ($1, $2, $3, $4, $5)`,
		actor, action, target, ip, d)
	return err
}

// ListAudit returns recent audit entries, newest first.
func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, ts, COALESCE(actor,''), action, COALESCE(target,''), COALESCE(ip,''), detail
		FROM audit_log ORDER BY ts DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var detailRaw []byte
		if err := rows.Scan(&e.ID, &e.TS, &e.Actor, &e.Action, &e.Target, &e.IP, &detailRaw); err != nil {
			return nil, err
		}
		e.Detail = map[string]any{}
		if len(detailRaw) > 0 {
			_ = json.Unmarshal(detailRaw, &e.Detail)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
