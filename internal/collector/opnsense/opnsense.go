// Package opnsense implements the OPNsense collector via the OPNsense REST API
// (HTTP Basic auth with an API key/secret over HTTPS).
package opnsense

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/stock3/motzworks/internal/collector"
	"github.com/stock3/motzworks/internal/model"
)

// Collector gathers inventory from an OPNsense firewall.
type Collector struct {
	log      *slog.Logger
	Port     int
	Insecure bool
	Timeout  time.Duration
}

// New returns an OPNsense collector (HTTPS/443, TLS-insecure by default).
func New(log *slog.Logger) *Collector {
	return &Collector{log: log, Port: 443, Insecure: true, Timeout: 15 * time.Second}
}

func (c *Collector) Name() string { return "opnsense" }

func (c *Collector) Supports(class collector.DeviceClass) bool {
	return class == collector.ClassFirewall || class == collector.ClassUnknown
}

// pickCredential returns the first OPNsense API credential (key=Username,
// secret=Secret).
func pickCredential(creds []collector.Credential) (collector.Credential, bool) {
	for _, cr := range creds {
		if cr.Kind == "opnsense-api" {
			return cr, true
		}
	}
	return collector.Credential{}, false
}

// firmwareStatus covers the product fields of /api/core/firmware/status across
// OPNsense versions (top-level and nested "product").
type firmwareStatus struct {
	ProductName    string `json:"product_name"`
	ProductVersion string `json:"product_version"`
	Product        struct {
		ProductName    string `json:"product_name"`
		ProductVersion string `json:"product_version"`
	} `json:"product"`
}

func (f firmwareStatus) name() string {
	if f.Product.ProductName != "" {
		return f.Product.ProductName
	}
	return f.ProductName
}

func (f firmwareStatus) version() string {
	if f.Product.ProductVersion != "" {
		return f.Product.ProductVersion
	}
	return f.ProductVersion
}

// parseFirmware maps /api/core/firmware/status into a device, or errors if the
// payload isn't recognizably OPNsense (self-identification).
func parseFirmware(raw []byte) (model.Device, error) {
	var fw firmwareStatus
	if err := json.Unmarshal(raw, &fw); err != nil {
		return model.Device{}, err
	}
	if !strings.Contains(strings.ToLower(fw.name()), "opnsense") {
		return model.Device{}, errors.New("not an OPNsense endpoint")
	}
	return model.Device{
		Type:   model.TypeFirewall,
		Source: "opnsense",
		OS: &model.OSInfo{
			Family:  "opnsense",
			Name:    fw.name(),
			Version: fw.version(),
		},
	}, nil
}

func (c *Collector) Collect(ctx context.Context, t collector.Target) (collector.Result, error) {
	cred, ok := pickCredential(t.Credentials)
	if !ok {
		return collector.Result{}, errors.New("no opnsense-api credential")
	}
	port := c.Port
	if port == 0 {
		port = 443
	}
	base := fmt.Sprintf("https://%s:%d", t.Addr.String(), port)

	client := &http.Client{
		Timeout:   c.Timeout,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: c.Insecure}},
	}
	raw, err := apiGetBasic(ctx, client, base+"/api/core/firmware/status", cred.Username, cred.Secret)
	if err != nil {
		return collector.Result{}, fmt.Errorf("opnsense firmware status: %w", err)
	}
	dev, err := parseFirmware(raw)
	if err != nil {
		return collector.Result{}, err
	}
	dev.PrimaryIP = t.Addr
	return collector.Result{Target: t, Device: dev, Raw: map[string]any{"firmware": string(raw)}}, nil
}

func apiGetBasic(ctx context.Context, client *http.Client, url, user, pass string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return body, nil
}
