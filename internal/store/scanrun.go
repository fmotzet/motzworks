package store

import "context"

// CreateScanRun starts a scan_run row and returns its id. scanTargetID may be
// nil for ad-hoc scans; specs are the CIDR/IP/range strings being scanned.
func (s *Store) CreateScanRun(ctx context.Context, scanTargetID *string, specs []string) (string, error) {
	if specs == nil {
		specs = []string{}
	}
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO scan_run (scan_target_id, status, targets) VALUES ($1, 'running', $2) RETURNING id`,
		scanTargetID, specs,
	).Scan(&id)
	return id, err
}

// SetScanDiscovered records how many live hosts a run will process.
func (s *Store) SetScanDiscovered(ctx context.Context, id string, n int) error {
	_, err := s.pool.Exec(ctx, `UPDATE scan_run SET discovered = $2 WHERE id = $1`, id, n)
	return err
}

// InsertScanEvent appends a per-host progress event for a scan run.
func (s *Store) InsertScanEvent(ctx context.Context, runID, addr, class, collector, status string, changes int, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO scan_event (scan_run_id, addr, device_class, collector, status, changes, error)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7,''))`,
		runID, addr, class, collector, status, changes, errMsg)
	return err
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
