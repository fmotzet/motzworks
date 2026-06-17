package store

import "context"

// CreateScanRun starts a scan_run row and returns its id. scanTargetID may be
// nil for ad-hoc CLI scans not tied to a stored target.
func (s *Store) CreateScanRun(ctx context.Context, scanTargetID *string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO scan_run (scan_target_id, status) VALUES ($1, 'running') RETURNING id`,
		scanTargetID,
	).Scan(&id)
	return id, err
}

// FinishScanRun records the terminal state of a scan run. A non-empty errMsg
// is stored and the status forced to 'failed'.
func (s *Store) FinishScanRun(ctx context.Context, id, status string, hostsFound int, errMsg string) error {
	if errMsg != "" {
		status = "failed"
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE scan_run
		SET finished_at = now(), status = $2, hosts_found = $3, error = NULLIF($4,'')
		WHERE id = $1`,
		id, status, hostsFound, errMsg)
	return err
}
