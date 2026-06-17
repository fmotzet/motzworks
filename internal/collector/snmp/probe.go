package snmp

import (
	"context"
	"net/netip"

	"github.com/stock3/motzworks/internal/collector"
	"github.com/stock3/motzworks/internal/discovery"
	"github.com/stock3/motzworks/internal/fingerprint"
	"github.com/stock3/motzworks/internal/worker"
)

// Probe checks each address for an SNMP agent (a sysDescr.0 GET) and returns
// those that respond as discovery hosts with the SNMP port marked open, so the
// scan engine can find managed devices that expose no TCP services. concurrency
// caps simultaneous probes.
func (c *Collector) Probe(ctx context.Context, addrs []netip.Addr, cred collector.Credential, concurrency int) []discovery.Host {
	if concurrency <= 0 {
		concurrency = 64
	}
	jobs := make([]worker.Job[*discovery.Host], len(addrs))
	for i, a := range addrs {
		a := a
		jobs[i] = func(ctx context.Context) (*discovery.Host, error) {
			g, err := c.client(ctx, a.String(), cred)
			if err != nil {
				return nil, nil
			}
			defer g.Conn.Close()
			if _, err := g.Get([]string{oidSysDescr}); err != nil {
				return nil, nil
			}
			return &discovery.Host{Addr: a, OpenPorts: []int{fingerprint.SNMPPort}}, nil
		}
	}

	results := worker.Run(ctx, concurrency, jobs)
	var hosts []discovery.Host
	for _, r := range results {
		if r.Value != nil {
			hosts = append(hosts, *r.Value)
		}
	}
	return hosts
}
