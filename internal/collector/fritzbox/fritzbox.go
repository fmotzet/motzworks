// Package fritzbox identifies and inventories AVM FRITZ!Box gateways via their
// unauthenticated TR-064 device description (http://<ip>:49000/tr64desc.xml).
//
// Unlike the OPNsense/FortiGate collectors, this needs no credential: the
// device-description XML is served without authentication and already carries
// manufacturer, model, firmware version and serial. That keeps the end-user
// configuration empty for the common home-gateway case. Richer data (connected
// LAN hosts, WAN/DSL status, the true AVM serial) requires authenticated TR-064
// SOAP calls and is left for a later, credentialed tier.
package fritzbox

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stock3/motzworks/internal/collector"
	"github.com/stock3/motzworks/internal/model"
)

// TR064Port is the conventional TR-064 (HTTP) port on a FRITZ!Box.
const TR064Port = 49000

// descPath is the unauthenticated TR-064 device-description document.
const descPath = "/tr64desc.xml"

// Collector probes a host's TR-064 device description and claims it only when
// the manufacturer is AVM.
type Collector struct {
	log     *slog.Logger
	Port    int
	Timeout time.Duration
}

// New returns a FRITZ!Box collector with sensible defaults.
func New(log *slog.Logger) *Collector {
	return &Collector{log: log, Port: TR064Port, Timeout: 5 * time.Second}
}

func (c *Collector) Name() string { return "fritzbox" }

// Supports handles unknown hosts (a FRITZ!Box with only 80/443/49000 open
// fingerprints as unknown) and the firewall/router classes for completeness.
func (c *Collector) Supports(class collector.DeviceClass) bool {
	return class == collector.ClassUnknown || class == collector.ClassFirewall
}

// tr64Desc is the subset of the TR-064 device description we consume. The
// document uses a default XML namespace; encoding/xml matches by local name.
type tr64Desc struct {
	XMLName       xml.Name `xml:"root"`
	SystemVersion struct {
		Display string `xml:"Display"`
	} `xml:"systemVersion"`
	Device struct {
		FriendlyName     string `xml:"friendlyName"`
		Manufacturer     string `xml:"manufacturer"`
		ModelName        string `xml:"modelName"`
		ModelDescription string `xml:"modelDescription"`
		SerialNumber     string `xml:"serialNumber"`
	} `xml:"device"`
}

// parseDesc turns a TR-064 device description into a Device, rejecting anything
// that does not self-identify as AVM. Rejection returns ErrNoCredential so the
// engine leaves a non-FRITZ!Box host discovered-only rather than marking it
// failed (this collector probes every unknown host, so non-matches are normal).
func parseDesc(raw []byte) (model.Device, error) {
	var d tr64Desc
	if err := xml.Unmarshal(raw, &d); err != nil {
		return model.Device{}, collector.ErrNoCredential
	}
	if !strings.EqualFold(strings.TrimSpace(d.Device.Manufacturer), "AVM") {
		return model.Device{}, collector.ErrNoCredential
	}

	name := firstNonEmpty(d.Device.ModelName, d.Device.FriendlyName, d.Device.ModelDescription)
	dev := model.Device{
		Type:   model.TypeRouter,
		Source: "fritzbox",
		Hardware: &model.Hardware{
			Vendor: "AVM",
			Model:  name,
			Serial: strings.TrimSpace(d.Device.SerialNumber),
		},
	}
	if v := strings.TrimSpace(d.SystemVersion.Display); v != "" {
		dev.OS = &model.OSInfo{Family: "fritzos", Name: name, Version: v}
	}

	// The UDN serialNumber is the gateway's burned-in hardware MAC. Recording it
	// as an interface gives the device a stable identity key for scheduled
	// rescans (it ranks above primary IP), surviving a DHCP/IP change. Only used
	// when it parses as a real MAC.
	if mac, ok := parseMAC(d.Device.SerialNumber); ok {
		dev.Interfaces = []model.Interface{{Name: "lan", MAC: mac}}
	}
	return dev, nil
}

// parseMAC returns the normalized MAC if s is a valid 6-octet hardware address.
func parseMAC(s string) (string, bool) {
	hw, err := net.ParseMAC(strings.TrimSpace(s))
	if err != nil || len(hw) != 6 {
		return "", false
	}
	return hw.String(), true
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func (c *Collector) Collect(ctx context.Context, t collector.Target) (collector.Result, error) {
	port := c.Port
	if port == 0 {
		port = TR064Port
	}
	url := "http://" + net.JoinHostPort(t.Addr.String(), strconv.Itoa(port)) + descPath

	raw, err := c.fetch(ctx, url)
	if err != nil {
		// Unreachable TR-064 endpoint → not a FRITZ!Box we can read; treat as
		// not-applicable so the host stays discovered-only.
		return collector.Result{}, collector.ErrNoCredential
	}
	dev, err := parseDesc(raw)
	if err != nil {
		return collector.Result{}, err
	}
	dev.PrimaryIP = t.Addr

	return collector.Result{
		Target: t,
		Device: dev,
		Raw:    map[string]any{"tr64desc": string(raw)},
	}, nil
}

func (c *Collector) fetch(ctx context.Context, url string) ([]byte, error) {
	client := &http.Client{Timeout: c.Timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}
