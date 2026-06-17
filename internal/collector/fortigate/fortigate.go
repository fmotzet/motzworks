// Package fortigate implements the FortiGate collector via the FortiOS REST API
// (Bearer API token over HTTPS).
package fortigate

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

// Collector gathers inventory from a FortiGate firewall.
type Collector struct {
	log      *slog.Logger
	Port     int
	Insecure bool
	Timeout  time.Duration
}

// New returns a FortiGate collector (HTTPS/443, TLS-insecure by default).
func New(log *slog.Logger) *Collector {
	return &Collector{log: log, Port: 443, Insecure: true, Timeout: 15 * time.Second}
}

func (c *Collector) Name() string { return "fortigate" }

func (c *Collector) Supports(class collector.DeviceClass) bool {
	return class == collector.ClassFirewall || class == collector.ClassUnknown
}

// pickCredential returns the first FortiGate API token (token=Secret).
func pickCredential(creds []collector.Credential) (collector.Credential, bool) {
	for _, cr := range creds {
		if cr.Kind == "fortigate-token" {
			return cr, true
		}
	}
	return collector.Credential{}, false
}

// systemStatus covers /api/v2/monitor/system/status. FortiOS puts identity
// fields at the top level alongside a "results" object.
type systemStatus struct {
	Serial   string `json:"serial"`
	Version  string `json:"version"`
	Build    int    `json:"build"`
	Hostname string `json:"hostname"`
	Model    string `json:"model"`
	Results  struct {
		Hostname  string `json:"hostname"`
		Model     string `json:"model"`
		ModelName string `json:"model_name"`
	} `json:"results"`
}

// parseStatus maps system status into a device, or errors if it isn't a
// recognizable FortiGate response (self-identification).
func parseStatus(raw []byte) (model.Device, error) {
	var s systemStatus
	if err := json.Unmarshal(raw, &s); err != nil {
		return model.Device{}, err
	}
	if s.Serial == "" && !strings.HasPrefix(strings.ToLower(s.Version), "v") {
		return model.Device{}, errors.New("not a FortiGate endpoint")
	}
	hostname := s.Hostname
	if hostname == "" {
		hostname = s.Results.Hostname
	}
	model_ := s.Model
	if model_ == "" {
		model_ = s.Results.Model
	}
	dev := model.Device{
		Type:     model.TypeFirewall,
		Source:   "fortigate",
		Hostname: hostname,
		Serial:   s.Serial,
		OS:       &model.OSInfo{Family: "fortios", Name: "FortiOS", Version: s.Version},
	}
	if model_ != "" || s.Serial != "" {
		dev.Hardware = &model.Hardware{Vendor: "Fortinet", Model: model_, Serial: s.Serial}
	}
	return dev, nil
}

func (c *Collector) Collect(ctx context.Context, t collector.Target) (collector.Result, error) {
	cred, ok := pickCredential(t.Credentials)
	if !ok {
		return collector.Result{}, collector.ErrNoCredential
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
	raw, err := apiGetBearer(ctx, client, base+"/api/v2/monitor/system/status", cred.Secret)
	if err != nil {
		return collector.Result{}, fmt.Errorf("fortigate system status: %w", err)
	}
	dev, err := parseStatus(raw)
	if err != nil {
		return collector.Result{}, err
	}
	dev.PrimaryIP = t.Addr
	return collector.Result{Target: t, Device: dev, Raw: map[string]any{"status": string(raw)}}, nil
}

func apiGetBearer(ctx context.Context, client *http.Client, url, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
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
