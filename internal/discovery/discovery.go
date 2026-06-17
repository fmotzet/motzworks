// Package discovery finds live hosts on the network. It expands scan specs
// (CIDRs, single IPs, ranges) into addresses and probes them with TCP connect
// attempts on a set of common ports — no raw sockets or privileges required.
// The set of open ports doubles as a hint for fingerprinting.
package discovery

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/stock3/motzworks/internal/worker"
)

// DefaultTCPPorts are probed to detect liveness and hint at OS/role.
//
//	22   ssh        135  msrpc      139/445 smb     161 (udp, separate)
//	443  https      3389 rdp        5985/5986 winrm 8006 proxmox
//	9100 jetdirect  631  ipp        515  lpd        80   http
//	49000 TR-064 (AVM FRITZ!Box gateway)
var DefaultTCPPorts = []int{22, 80, 135, 139, 443, 445, 515, 631, 3389, 5985, 5986, 8006, 9100, 49000}

// Host is a discovered live host and the ports found open on it.
type Host struct {
	Addr      netip.Addr
	OpenPorts []int
}

// HasPort reports whether port was found open.
func (h Host) HasPort(port int) bool {
	for _, p := range h.OpenPorts {
		if p == port {
			return true
		}
	}
	return false
}

// Options tune a discovery sweep.
type Options struct {
	TCPPorts    []int
	Timeout     time.Duration // per-connection timeout
	Concurrency int           // hosts probed concurrently

	// Politeness controls for large networks (0 = disabled):
	RatePerSec int           // global cap on hosts probed per second
	Jitter     time.Duration // random delay [0,Jitter) before each host probe
}

func (o Options) withDefaults() Options {
	if len(o.TCPPorts) == 0 {
		o.TCPPorts = DefaultTCPPorts
	}
	if o.Timeout <= 0 {
		o.Timeout = 2 * time.Second
	}
	if o.Concurrency <= 0 {
		o.Concurrency = 256
	}
	return o
}

// Expand parses scan specs into a deduplicated list of addresses. Supported
// forms: "10.0.0.5" (single), "10.0.0.0/24" (CIDR), "10.0.0.1-10.0.0.50"
// (inclusive range). IPv6 single/CIDR are supported; IPv6 ranges are not.
func Expand(specs []string) ([]netip.Addr, error) {
	seen := make(map[netip.Addr]struct{})
	var out []netip.Addr
	add := func(a netip.Addr) {
		if _, ok := seen[a]; ok {
			return
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}

	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		switch {
		case strings.Contains(spec, "/"):
			prefix, err := netip.ParsePrefix(spec)
			if err != nil {
				return nil, fmt.Errorf("invalid CIDR %q: %w", spec, err)
			}
			prefix = prefix.Masked()
			for a := prefix.Addr(); prefix.Contains(a); a = a.Next() {
				add(a)
			}
		case strings.Contains(spec, "-"):
			lo, hi, err := parseRange(spec)
			if err != nil {
				return nil, err
			}
			for a := lo; ; a = a.Next() {
				add(a)
				if a == hi {
					break
				}
			}
		default:
			a, err := netip.ParseAddr(spec)
			if err != nil {
				return nil, fmt.Errorf("invalid address %q: %w", spec, err)
			}
			add(a)
		}
	}
	return out, nil
}

func parseRange(spec string) (lo, hi netip.Addr, err error) {
	parts := strings.SplitN(spec, "-", 2)
	lo, err = netip.ParseAddr(strings.TrimSpace(parts[0]))
	if err != nil {
		return lo, hi, fmt.Errorf("invalid range start in %q: %w", spec, err)
	}
	hi, err = netip.ParseAddr(strings.TrimSpace(parts[1]))
	if err != nil {
		return lo, hi, fmt.Errorf("invalid range end in %q: %w", spec, err)
	}
	if lo.Is4() != hi.Is4() || lo.Compare(hi) > 0 {
		return lo, hi, fmt.Errorf("invalid range %q", spec)
	}
	return lo, hi, nil
}

// Discover probes addresses and returns those with at least one open port.
func Discover(ctx context.Context, addrs []netip.Addr, opts Options) []Host {
	opts = opts.withDefaults()

	lim := newLimiter(opts.RatePerSec)
	defer lim.close()

	jobs := make([]worker.Job[Host], len(addrs))
	for i, a := range addrs {
		a := a
		jobs[i] = func(ctx context.Context) (Host, error) {
			lim.wait(ctx)
			if opts.Jitter > 0 {
				sleep(ctx, time.Duration(rand.Int64N(int64(opts.Jitter))))
			}
			return probe(ctx, a, opts), nil
		}
	}

	results := worker.Run(ctx, opts.Concurrency, jobs)
	var live []Host
	for _, r := range results {
		if len(r.Value.OpenPorts) > 0 {
			live = append(live, r.Value)
		}
	}
	return live
}

// sleep waits for d or until ctx is cancelled.
func sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

// limiter is a simple token-bucket rate limiter shared across discovery workers.
type limiter struct {
	tokens chan struct{}
	stop   chan struct{}
}

// newLimiter returns a limiter emitting perSec tokens per second, or nil
// (unlimited) when perSec <= 0.
func newLimiter(perSec int) *limiter {
	if perSec <= 0 {
		return nil
	}
	l := &limiter{tokens: make(chan struct{}, perSec), stop: make(chan struct{})}
	go func() {
		t := time.NewTicker(time.Second / time.Duration(perSec))
		defer t.Stop()
		for {
			select {
			case <-l.stop:
				return
			case <-t.C:
				select {
				case l.tokens <- struct{}{}:
				default: // bucket full
				}
			}
		}
	}()
	return l
}

func (l *limiter) wait(ctx context.Context) {
	if l == nil {
		return
	}
	select {
	case <-l.tokens:
	case <-ctx.Done():
	}
}

func (l *limiter) close() {
	if l != nil {
		close(l.stop)
	}
}

func probe(ctx context.Context, a netip.Addr, opts Options) Host {
	h := Host{Addr: a}
	for _, port := range opts.TCPPorts {
		if ctx.Err() != nil {
			break
		}
		d := net.Dialer{Timeout: opts.Timeout}
		conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(a.String(), strconv.Itoa(port)))
		if err == nil {
			_ = conn.Close()
			h.OpenPorts = append(h.OpenPorts, port)
		}
	}
	return h
}
