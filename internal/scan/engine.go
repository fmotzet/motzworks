// Package scan is the orchestration engine: it expands scan specs, discovers
// live hosts, fingerprints them, runs the matching collectors with bounded
// concurrency, and persists normalized devices (with change tracking) to the
// store.
package scan

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"

	"github.com/stock3/motzworks/internal/collector"
	"github.com/stock3/motzworks/internal/discovery"
	"github.com/stock3/motzworks/internal/fingerprint"
	"github.com/stock3/motzworks/internal/model"
	"github.com/stock3/motzworks/internal/store"
	"github.com/stock3/motzworks/internal/worker"
)

// Engine runs scans end-to-end.
type Engine struct {
	store *store.Store
	reg   *collector.Registry
	log   *slog.Logger
}

// New constructs an Engine.
func New(st *store.Store, reg *collector.Registry, log *slog.Logger) *Engine {
	return &Engine{store: st, reg: reg, log: log}
}

// Options parameterize a single scan run.
type Options struct {
	Specs          []string
	Credentials    []collector.Credential
	Discovery      discovery.Options
	CollectWorkers int

	// Probe, if set, discovers additional hosts out-of-band (e.g. via an SNMP
	// UDP probe) over the expanded address list; results are merged with the
	// TCP discovery results.
	Probe func(ctx context.Context, addrs []netip.Addr) []discovery.Host
}

// HostResult is the outcome for a single host.
type HostResult struct {
	Addr       string
	Class      collector.DeviceClass
	DeviceID   string
	Collector  string
	Changes    int
	Err        error  // persistence error
	CollectErr string // last collector error, if no collector succeeded
}

// Summary aggregates a scan run.
type Summary struct {
	ScanRunID  string
	Discovered int
	Collected  int
	Persisted  int
	Hosts      []HostResult
}

// Run performs discovery, collection and persistence for the given options.
func (e *Engine) Run(ctx context.Context, opts Options) (Summary, error) {
	addrs, err := discovery.Expand(opts.Specs)
	if err != nil {
		return Summary{}, err
	}

	runID, err := e.store.CreateScanRun(ctx, nil)
	if err != nil {
		return Summary{}, err
	}

	e.log.Info("discovery started", "addresses", len(addrs))
	var extra []discovery.Host
	if opts.Probe != nil {
		extra = opts.Probe(ctx, addrs)
	}
	live := mergeHosts(discovery.Discover(ctx, addrs, opts.Discovery), extra)
	e.log.Info("discovery finished", "live", len(live), "snmp_probe", len(extra))
	if err := e.store.SetScanDiscovered(ctx, runID, len(live)); err != nil {
		e.log.Warn("set discovered count failed", "err", err)
	}

	workers := opts.CollectWorkers
	if workers <= 0 {
		workers = 32
	}

	jobs := make([]worker.Job[HostResult], len(live))
	for i, h := range live {
		h := h
		jobs[i] = func(ctx context.Context) (HostResult, error) {
			return e.processHost(ctx, runID, h, opts.Credentials), nil
		}
	}
	results := worker.Run(ctx, workers, jobs)

	sum := Summary{ScanRunID: runID, Discovered: len(live)}
	for _, r := range results {
		hr := r.Value
		sum.Hosts = append(sum.Hosts, hr)
		if hr.Collector != "" && hr.Err == nil {
			sum.Collected++
		}
		if hr.DeviceID != "" {
			sum.Persisted++
		}
	}

	if err := e.store.FinishScanRun(ctx, runID, "ok", sum.Persisted, ""); err != nil {
		return sum, err
	}
	e.log.Info("scan finished",
		"discovered", sum.Discovered, "collected", sum.Collected, "persisted", sum.Persisted)
	return sum, nil
}

func (e *Engine) processHost(ctx context.Context, runID string, h discovery.Host, creds []collector.Credential) HostResult {
	class := fingerprint.ClassifyByPorts(h.OpenPorts)
	hr := HostResult{Addr: h.Addr.String(), Class: class}

	// Base device from discovery alone; a successful collector enriches it.
	dev := model.Device{
		Type:      fingerprint.ClassToType(class),
		PrimaryIP: h.Addr,
		Source:    "discovery",
	}

	var related []collector.Related
	for _, c := range e.reg.For(class) {
		t := collector.Target{Addr: h.Addr, Hostname: h.Addr.String(), Class: class, Credentials: creds}
		res, err := c.Collect(ctx, t)
		if err != nil {
			if errors.Is(err, collector.ErrNoCredential) {
				continue // collector simply doesn't apply to this host
			}
			e.log.Debug("collector failed", "collector", c.Name(), "addr", hr.Addr, "err", err)
			hr.CollectErr = c.Name() + ": " + err.Error()
			continue
		}
		dev = mergeDevice(dev, res.Device, c.Name())
		related = res.Related
		hr.Collector = c.Name()
		hr.CollectErr = ""
		break
	}

	collected := hr.Collector != ""
	id, changes, err := e.store.UpsertDevice(ctx, dev, runID, collected)
	if err != nil {
		hr.Err = err
		e.recordEvent(ctx, runID, hr)
		return hr
	}
	hr.DeviceID = id
	hr.Changes = len(changes)
	e.recordEvent(ctx, runID, hr)

	// Persist related devices (e.g. VMs) and link them to this device.
	for _, rel := range related {
		childID, _, err := e.store.UpsertDevice(ctx, rel.Device, runID, true)
		if err != nil {
			e.log.Warn("upsert related device failed", "parent", hr.Addr, "err", err)
			continue
		}
		if err := e.store.CreateRelationship(ctx, id, childID, rel.Kind); err != nil {
			e.log.Warn("create relationship failed", "parent", id, "child", childID, "err", err)
		}
	}
	return hr
}

// recordEvent writes a per-host progress event for live scan monitoring.
func (e *Engine) recordEvent(ctx context.Context, runID string, hr HostResult) {
	status := "discovered"
	errMsg := ""
	switch {
	case hr.Err != nil:
		status, errMsg = "failed", hr.Err.Error()
	case hr.Collector != "":
		status = "collected"
	case hr.CollectErr != "":
		status, errMsg = "failed", hr.CollectErr
	}
	if err := e.store.InsertScanEvent(ctx, runID, hr.Addr, string(hr.Class), hr.Collector, status, hr.Changes, errMsg); err != nil {
		e.log.Debug("record scan event failed", "addr", hr.Addr, "err", err)
	}
}

// mergeDevice overlays collector output onto the discovery base: the discovered
// IP is preserved, the discovery type is a fallback, and the source is tagged.
func mergeDevice(base, collected model.Device, source string) model.Device {
	if !collected.PrimaryIP.IsValid() {
		collected.PrimaryIP = base.PrimaryIP
	}
	if collected.Type == "" || collected.Type == model.TypeUnknown {
		collected.Type = base.Type
	}
	if collected.Source == "" {
		collected.Source = source
	}
	return collected
}

// mergeHosts unions TCP-discovered hosts with extra hosts (e.g. SNMP probe
// responders), combining open-port sets for any address present in both.
func mergeHosts(primary, extra []discovery.Host) []discovery.Host {
	byAddr := make(map[string]*discovery.Host)
	var order []string
	add := func(h discovery.Host) {
		key := h.Addr.String()
		if cur, ok := byAddr[key]; ok {
			for _, p := range h.OpenPorts {
				if !cur.HasPort(p) {
					cur.OpenPorts = append(cur.OpenPorts, p)
				}
			}
			return
		}
		hc := h
		byAddr[key] = &hc
		order = append(order, key)
	}
	for _, h := range primary {
		add(h)
	}
	for _, h := range extra {
		add(h)
	}
	out := make([]discovery.Host, 0, len(order))
	for _, k := range order {
		out = append(out, *byAddr[k])
	}
	return out
}
