package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/stock3/motzworks/internal/collector"
	"github.com/stock3/motzworks/internal/collector/snmp"
	"github.com/stock3/motzworks/internal/config"
	"github.com/stock3/motzworks/internal/discovery"
	"github.com/stock3/motzworks/internal/logging"
	"github.com/stock3/motzworks/internal/scan"
	"github.com/stock3/motzworks/internal/store"
)

// cmdScan implements: motzworks scan -targets <specs> [credential flags]
func cmdScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	cfgPath := fs.String("config", "config.yaml", "config file")
	targets := fs.String("targets", "", "comma-separated CIDRs / IPs / ranges to scan")
	ports := fs.String("ports", "", "comma-separated TCP ports to probe (default: built-in set)")

	sshUser := fs.String("ssh-user", "", "SSH username")
	sshPass := fs.String("ssh-pass", "", "SSH password")
	sshKey := fs.String("ssh-key", "", "path to SSH private key (PEM)")
	snmpCommunity := fs.String("snmp-community", "", "SNMPv2c community string")
	winrmUser := fs.String("winrm-user", "", "WinRM username")
	winrmPass := fs.String("winrm-pass", "", "WinRM password")

	timeout := fs.Duration("timeout", 2*time.Second, "per-connection probe timeout")
	discoverConc := fs.Int("discover-concurrency", 256, "discovery concurrency")
	collectConc := fs.Int("collect-concurrency", 32, "collection concurrency")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *targets == "" {
		return errors.New("scan requires -targets (e.g. -targets 10.0.0.0/24)")
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	log := logging.New(cfg.Log.Level, cfg.Log.Format)

	tcpPorts, err := parsePorts(*ports)
	if err != nil {
		return err
	}

	creds, err := buildCredentials(credFlags{
		sshUser: *sshUser, sshPass: *sshPass, sshKey: *sshKey,
		snmpCommunity: *snmpCommunity, winrmUser: *winrmUser, winrmPass: *winrmPass,
	})
	if err != nil {
		return err
	}

	ctx := context.Background()
	st, err := store.Open(ctx, cfg.Database.DSN())
	if err != nil {
		return err
	}
	defer st.Close()

	reg := collector.NewRegistry()
	registerCollectors(reg, log)

	eng := scan.New(st, reg, log)
	opts := scan.Options{
		Specs:       splitNonEmpty(*targets),
		Credentials: creds,
		Discovery: discovery.Options{
			TCPPorts:    tcpPorts,
			Timeout:     *timeout,
			Concurrency: *discoverConc,
		},
		CollectWorkers: *collectConc,
	}
	// When an SNMP community is supplied, also probe UDP/161 during discovery
	// so network-only gear (no open TCP ports) is found.
	if cr, ok := snmpCredential(creds); ok {
		probe := snmp.New(log)
		opts.Probe = func(ctx context.Context, addrs []netip.Addr) []discovery.Host {
			return probe.Probe(ctx, addrs, cr, *discoverConc)
		}
	}
	sum, err := eng.Run(ctx, opts)
	if err != nil {
		return err
	}

	printSummary(sum)
	return nil
}

type credFlags struct {
	sshUser, sshPass, sshKey string
	snmpCommunity            string
	winrmUser, winrmPass     string
}

// buildCredentials turns scan flags into collector credentials. (Phase 2 will
// source these from the encrypted vault instead.)
func buildCredentials(f credFlags) ([]collector.Credential, error) {
	var creds []collector.Credential
	if f.sshUser != "" {
		c := collector.Credential{Kind: "ssh-password", Username: f.sshUser, Secret: f.sshPass}
		if f.sshKey != "" {
			key, err := os.ReadFile(f.sshKey)
			if err != nil {
				return nil, fmt.Errorf("read ssh key: %w", err)
			}
			c.Kind = "ssh-key"
			c.Secret = string(key)
		}
		creds = append(creds, c)
	}
	if f.snmpCommunity != "" {
		creds = append(creds, collector.Credential{Kind: "snmp-v2c", Secret: f.snmpCommunity})
	}
	if f.winrmUser != "" {
		creds = append(creds, collector.Credential{Kind: "winrm", Username: f.winrmUser, Secret: f.winrmPass})
	}
	return creds, nil
}

// snmpCredential returns the first SNMP credential in the set, if any.
func snmpCredential(creds []collector.Credential) (collector.Credential, bool) {
	for _, c := range creds {
		if c.Kind == "snmp-v2c" || c.Kind == "snmp-v3" {
			return c, true
		}
	}
	return collector.Credential{}, false
}

func parsePorts(s string) ([]int, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil // engine falls back to discovery.DefaultTCPPorts
	}
	var ports []int
	for _, p := range splitNonEmpty(s) {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid port %q", p)
		}
		ports = append(ports, n)
	}
	return ports, nil
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func printSummary(sum scan.Summary) {
	fmt.Printf("scan %s: discovered=%d collected=%d persisted=%d\n",
		sum.ScanRunID, sum.Discovered, sum.Collected, sum.Persisted)
	for _, h := range sum.Hosts {
		status := "ok"
		if h.Err != nil {
			status = "error: " + h.Err.Error()
		}
		via := h.Collector
		if via == "" {
			via = "(discovery only)"
			if h.CollectErr != "" {
				status = "collect failed: " + h.CollectErr
			}
		}
		fmt.Printf("  %-15s %-10s via=%-12s changes=%d %s\n",
			h.Addr, h.Class, via, h.Changes, status)
	}
}
