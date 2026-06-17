// Package proxmox implements the Proxmox VE collector. It inventories the
// hypervisor (version, node CPU/RAM) and enumerates the VMs/containers it hosts
// as related devices linked by a "hosts-vm" relationship.
package proxmox

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/stock3/motzworks/internal/collector"
)

// Collector gathers inventory from a Proxmox VE API endpoint.
type Collector struct {
	log      *slog.Logger
	Port     int
	Insecure bool // Proxmox uses a self-signed cert by default
	Timeout  time.Duration
}

// New returns a Proxmox collector with sensible defaults (8006, TLS-insecure).
func New(log *slog.Logger) *Collector {
	return &Collector{log: log, Port: 8006, Insecure: true, Timeout: 15 * time.Second}
}

func (c *Collector) Name() string { return "proxmox" }

func (c *Collector) Supports(class collector.DeviceClass) bool {
	return class == collector.ClassHypervisor
}

// pickCredential returns the first Proxmox API token credential. Username is the
// token id "user@realm!tokenid"; Secret is the token UUID.
func pickCredential(creds []collector.Credential) (collector.Credential, bool) {
	for _, cr := range creds {
		if cr.Kind == "proxmox-token" {
			return cr, true
		}
	}
	return collector.Credential{}, false
}

// Collect queries the Proxmox API and returns the hypervisor device plus its VMs.
func (c *Collector) Collect(ctx context.Context, t collector.Target) (collector.Result, error) {
	cred, ok := pickCredential(t.Credentials)
	if !ok {
		return collector.Result{}, collector.ErrNoCredential
	}

	port := c.Port
	if port == 0 {
		port = 8006
	}
	base := fmt.Sprintf("https://%s:%d/api2/json", t.Addr.String(), port)
	authHeader := "PVEAPIToken=" + cred.Username + "=" + cred.Secret

	client := &http.Client{
		Timeout: c.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: c.Insecure},
		},
	}
	get := func(path string) ([]byte, error) {
		return apiGet(ctx, client, base+path, authHeader)
	}

	// version doubles as the self-identification probe.
	verRaw, err := get("/version")
	if err != nil {
		return collector.Result{}, fmt.Errorf("proxmox version: %w", err)
	}
	nodesRaw, err := get("/nodes")
	if err != nil {
		return collector.Result{}, fmt.Errorf("proxmox nodes: %w", err)
	}

	dev, err := buildHypervisor(verRaw, nodesRaw)
	if err != nil {
		return collector.Result{}, fmt.Errorf("parse proxmox: %w", err)
	}
	dev.PrimaryIP = t.Addr

	var related []collector.Related
	if vmsRaw, err := get("/cluster/resources?type=vm"); err == nil {
		related, _ = buildVMs(vmsRaw)
	} else {
		c.log.Debug("proxmox vm enumeration failed", "addr", t.Addr.String(), "err", err)
	}

	return collector.Result{
		Target:  t,
		Device:  dev,
		Related: related,
		Raw:     map[string]any{"version": string(verRaw)},
	}, nil
}

func apiGet(ctx context.Context, client *http.Client, url, authHeader string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", authHeader)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %s", strconv.Itoa(resp.StatusCode))
	}
	return body, nil
}
