// Package scheduler runs recurring scans. A background loop polls for due
// schedules, runs the scan for each target's CIDRs using the stored (vault-
// sealed) credentials, and advances each schedule's next run time.
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/stock3/motzworks/internal/scan"
	"github.com/stock3/motzworks/internal/store"
	"github.com/stock3/motzworks/internal/vault"
)

// Scheduler runs due scans on a fixed tick.
type Scheduler struct {
	store       *store.Store
	engine      *scan.Engine
	vault       *vault.Vault
	log         *slog.Logger
	tick        time.Duration
	concurrency int
}

// New constructs a Scheduler. tick is how often to check for due schedules.
func New(st *store.Store, eng *scan.Engine, v *vault.Vault, log *slog.Logger, tick time.Duration, concurrency int) *Scheduler {
	if tick <= 0 {
		tick = 30 * time.Second
	}
	if concurrency <= 0 {
		concurrency = 32
	}
	return &Scheduler{store: st, engine: eng, vault: v, log: log, tick: tick, concurrency: concurrency}
}

// Run blocks until ctx is cancelled, checking for due schedules each tick.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.tick)
	defer t.Stop()
	s.runDue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runDue(ctx)
		}
	}
}

func (s *Scheduler) runDue(ctx context.Context) {
	due, err := s.store.DueSchedules(ctx)
	if err != nil {
		s.log.Error("scheduler: list due", "err", err)
		return
	}
	if len(due) == 0 {
		return
	}

	stored, err := s.store.ListCredentials(ctx)
	if err != nil {
		s.log.Error("scheduler: list credentials", "err", err)
		return
	}
	creds := scan.DecryptCredentials(stored, s.vault)

	for _, d := range due {
		// Advance first so a long scan doesn't get re-triggered next tick.
		if err := s.store.AdvanceSchedule(ctx, d.ScheduleID, d.IntervalSecs); err != nil {
			s.log.Error("scheduler: advance", "schedule", d.ScheduleID, "err", err)
			continue
		}
		if len(d.CIDRs) == 0 {
			continue
		}
		s.log.Info("scheduler: running scan", "target", d.ScanTargetID, "cidrs", d.CIDRs)
		opts := scan.Options{
			Specs:          d.CIDRs,
			Credentials:    creds,
			CollectWorkers: s.concurrency,
			Probe:          scan.SNMPProbe(creds, s.concurrency, s.log),
		}
		if _, err := s.engine.Run(ctx, opts); err != nil {
			s.log.Error("scheduler: scan failed", "target", d.ScanTargetID, "err", err)
		}
	}
}
