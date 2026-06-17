// Package snmp implements the SNMP collector for network gear (switches,
// routers, printers, firewalls, UPS, etc.) and an SNMP liveness probe used to
// discover devices that expose no TCP services.
package snmp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/stock3/motzworks/internal/collector"
	"github.com/stock3/motzworks/internal/model"
)

// Standard MIB-2 / Entity-MIB OIDs.
const (
	oidSysDescr    = "1.3.6.1.2.1.1.1.0"
	oidSysObjectID = "1.3.6.1.2.1.1.2.0"
	oidSysName     = "1.3.6.1.2.1.1.5.0"
	oidSysLocation = "1.3.6.1.2.1.1.6.0"

	oidIfDescr       = "1.3.6.1.2.1.2.2.1.2" // ifDescr
	oidIfPhysAddress = "1.3.6.1.2.1.2.2.1.6" // ifPhysAddress (MAC)
	oidIfSpeed       = "1.3.6.1.2.1.2.2.1.5" // ifSpeed (bps)

	oidEntPhysMfg    = "1.3.6.1.2.1.47.1.1.1.1.12" // entPhysicalMfgName
	oidEntPhysModel  = "1.3.6.1.2.1.47.1.1.1.1.13" // entPhysicalModelName
	oidEntPhysSerial = "1.3.6.1.2.1.47.1.1.1.1.11" // entPhysicalSerialNum
)

// Collector gathers inventory over SNMP (v2c, with best-effort v3).
type Collector struct {
	log     *slog.Logger
	Port    uint16
	Timeout time.Duration
	Retries int
}

// New returns an SNMP collector with sensible defaults.
func New(log *slog.Logger) *Collector {
	return &Collector{log: log, Port: 161, Timeout: 3 * time.Second, Retries: 1}
}

func (c *Collector) Name() string { return "snmp" }

func (c *Collector) Supports(class collector.DeviceClass) bool {
	return class == collector.ClassSNMP || class == collector.ClassFirewall
}

// pickCredential returns the first SNMP credential.
func pickCredential(creds []collector.Credential) (collector.Credential, bool) {
	for _, cr := range creds {
		if cr.Kind == "snmp-v2c" || cr.Kind == "snmp-v3" {
			return cr, true
		}
	}
	return collector.Credential{}, false
}

// client builds and connects a GoSNMP session for the target.
func (c *Collector) client(ctx context.Context, target string, cred collector.Credential) (*gosnmp.GoSNMP, error) {
	port := c.Port
	if port == 0 {
		port = 161
	}
	g := &gosnmp.GoSNMP{
		Context:   ctx,
		Target:    target,
		Port:      port,
		Timeout:   c.Timeout,
		Retries:   c.Retries,
		MaxOids:   gosnmp.MaxOids,
		Transport: "udp",
	}
	switch cred.Kind {
	case "snmp-v2c", "":
		g.Version = gosnmp.Version2c
		g.Community = cred.Secret
		if g.Community == "" {
			g.Community = "public"
		}
	case "snmp-v3":
		g.Version = gosnmp.Version3
		g.SecurityModel = gosnmp.UserSecurityModel
		g.MsgFlags = gosnmp.AuthPriv
		g.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 cred.Username,
			AuthenticationProtocol:   gosnmp.SHA,
			AuthenticationPassphrase: cred.Extra["auth_pass"],
			PrivacyProtocol:          gosnmp.AES,
			PrivacyPassphrase:        cred.Extra["priv_pass"],
		}
	default:
		return nil, fmt.Errorf("unsupported snmp credential kind %q", cred.Kind)
	}
	if err := g.Connect(); err != nil {
		return nil, fmt.Errorf("snmp connect: %w", err)
	}
	return g, nil
}

// Collect queries the device and returns normalized inventory.
func (c *Collector) Collect(ctx context.Context, t collector.Target) (collector.Result, error) {
	cred, ok := pickCredential(t.Credentials)
	if !ok {
		return collector.Result{}, errors.New("no snmp credential")
	}

	g, err := c.client(ctx, t.Addr.String(), cred)
	if err != nil {
		return collector.Result{}, err
	}
	defer g.Conn.Close()

	scalars, err := g.Get([]string{oidSysDescr, oidSysObjectID, oidSysName, oidSysLocation})
	if err != nil {
		return collector.Result{}, fmt.Errorf("snmp get: %w", err)
	}

	dev := model.Device{Type: model.TypeSwitch, PrimaryIP: t.Addr, Source: "snmp"}
	raw := map[string]any{}

	var sysDescr string
	for _, pdu := range scalars.Variables {
		switch pdu.Name {
		case "." + oidSysDescr:
			sysDescr = pduString(pdu)
			raw["sysDescr"] = sysDescr
		case "." + oidSysObjectID:
			raw["sysObjectID"] = pduString(pdu)
		case "." + oidSysName:
			dev.Hostname = pduString(pdu)
		case "." + oidSysLocation:
			raw["sysLocation"] = pduString(pdu)
		}
	}
	if sysDescr != "" {
		dev.OS = &model.OSInfo{Family: osFamily(sysDescr), Name: firstLine(sysDescr)}
	}

	dev.Interfaces = c.collectInterfaces(g)
	dev.Hardware = c.collectHardware(g)

	return collector.Result{Target: t, Device: dev, Raw: raw}, nil
}

// collectInterfaces walks the ifTable and builds interfaces with MAC + speed.
func (c *Collector) collectInterfaces(g *gosnmp.GoSNMP) []model.Interface {
	descrs := walkColumn(g, oidIfDescr)
	macs := walkColumn(g, oidIfPhysAddress)
	speeds := walkColumn(g, oidIfSpeed)

	var ifaces []model.Interface
	for idx, d := range descrs {
		ifc := model.Interface{Name: pduString(d)}
		if m, ok := macs[idx]; ok {
			ifc.MAC = pduMAC(m)
		}
		if s, ok := speeds[idx]; ok {
			ifc.SpeedMbps = pduUint(s) / 1_000_000 // bps -> Mbps
		}
		if ifc.Name == "" && ifc.MAC == "" {
			continue
		}
		ifaces = append(ifaces, ifc)
	}
	return ifaces
}

// collectHardware reads the first Entity-MIB row that carries a serial number.
func (c *Collector) collectHardware(g *gosnmp.GoSNMP) *model.Hardware {
	serials := walkColumn(g, oidEntPhysSerial)
	models := walkColumn(g, oidEntPhysModel)
	mfgs := walkColumn(g, oidEntPhysMfg)

	for idx, s := range serials {
		serial := strings.TrimSpace(pduString(s))
		if serial == "" {
			continue
		}
		hw := &model.Hardware{Serial: serial}
		if m, ok := models[idx]; ok {
			hw.Model = strings.TrimSpace(pduString(m))
		}
		if m, ok := mfgs[idx]; ok {
			hw.Vendor = strings.TrimSpace(pduString(m))
		}
		return hw
	}
	return nil
}

// walkColumn BulkWalks a table column and returns PDUs keyed by row index (the
// OID suffix after the column base).
func walkColumn(g *gosnmp.GoSNMP, base string) map[string]gosnmp.SnmpPDU {
	out := map[string]gosnmp.SnmpPDU{}
	_ = g.BulkWalk(base, func(p gosnmp.SnmpPDU) error {
		idx := strings.TrimPrefix(p.Name, "."+base)
		idx = strings.TrimPrefix(idx, base)
		idx = strings.TrimPrefix(idx, ".")
		out[idx] = p
		return nil
	})
	return out
}

// pduString renders a PDU value as a string (handles OctetString bytes and
// ObjectIdentifier/integer forms).
func pduString(p gosnmp.SnmpPDU) string {
	switch v := p.Value.(type) {
	case []byte:
		return strings.TrimRight(string(v), "\x00")
	case string:
		return v
	case int, int64, uint, uint64:
		return fmt.Sprintf("%d", v)
	default:
		if p.Value == nil {
			return ""
		}
		return fmt.Sprintf("%v", p.Value)
	}
}

// pduUint coerces a numeric PDU value to uint64.
func pduUint(p gosnmp.SnmpPDU) int64 {
	switch v := p.Value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case uint:
		return int64(v)
	case uint64:
		return int64(v)
	case uint32:
		return int64(v)
	default:
		if s := pduString(p); s != "" {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return n
			}
		}
		return 0
	}
}

// pduMAC formats a 6-byte ifPhysAddress as a colon-separated MAC, returning ""
// for empty or all-zero addresses.
func pduMAC(p gosnmp.SnmpPDU) string {
	b, ok := p.Value.([]byte)
	if !ok || len(b) != 6 {
		return ""
	}
	allZero := true
	parts := make([]string, 6)
	for i, x := range b {
		if x != 0 {
			allZero = false
		}
		parts[i] = fmt.Sprintf("%02x", x)
	}
	if allZero {
		return ""
	}
	return strings.Join(parts, ":")
}

// osFamily guesses an OS family from sysDescr text.
func osFamily(sysDescr string) string {
	s := strings.ToLower(sysDescr)
	switch {
	case strings.Contains(s, "linux"):
		return "linux"
	case strings.Contains(s, "windows"):
		return "windows"
	case strings.Contains(s, "junos"):
		return "junos"
	case strings.Contains(s, "ios") || strings.Contains(s, "cisco"):
		return "cisco-ios"
	case strings.Contains(s, "fortios") || strings.Contains(s, "fortigate"):
		return "fortios"
	default:
		return ""
	}
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}
