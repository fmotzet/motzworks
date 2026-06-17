// Package collector defines the pluggable collector interface and a registry.
// Each collector knows how to gather inventory from one class of device
// (Windows via WinRM, Linux via SSH, network gear via SNMP, etc.).
package collector

import (
	"context"
	"net/netip"
	"sync"

	"github.com/stock3/motzworks/internal/model"
)

// DeviceClass is the fingerprinted classification used to route a target to
// the collectors that can handle it.
type DeviceClass string

const (
	ClassUnknown    DeviceClass = "unknown"
	ClassWindows    DeviceClass = "windows"
	ClassLinux      DeviceClass = "linux"
	ClassMac        DeviceClass = "mac"
	ClassSNMP       DeviceClass = "snmp"
	ClassHypervisor DeviceClass = "hypervisor"
	ClassFirewall   DeviceClass = "firewall"
)

// Credential is a decrypted credential handed to a collector at scan time.
// It is produced by the vault from a stored, sealed credential.
type Credential struct {
	ID       string
	Kind     string // ssh-password, ssh-key, winrm, snmp-v2c, snmp-v3, api-token
	Username string
	Secret   string
	Extra    map[string]string
}

// Target is a single host to collect from, along with candidate credentials.
type Target struct {
	Addr        netip.Addr
	Hostname    string
	Class       DeviceClass
	Credentials []Credential
}

// Result is the normalized output of collecting from one target. Raw holds the
// untouched collector payload for audit/debugging and is persisted as JSONB.
type Result struct {
	Target  Target
	Device  model.Device
	Raw     map[string]any
	Related []Related // additional devices discovered via this target
}

// Related is an additional device discovered through the primary target — e.g.
// a VM enumerated from its hypervisor — to be linked to the primary device by a
// relationship of the given Kind (e.g. "hosts-vm").
type Related struct {
	Device model.Device
	Kind   string
}

// Collector gathers inventory from a single class of device.
type Collector interface {
	// Name identifies the collector, e.g. "ssh", "snmp", "winrm".
	Name() string
	// Supports reports whether this collector can handle the given class.
	Supports(class DeviceClass) bool
	// Collect gathers inventory from a single target. Implementations must
	// honor ctx cancellation/deadlines.
	Collect(ctx context.Context, t Target) (Result, error)
}

// Registry holds the set of available collectors.
type Registry struct {
	mu         sync.RWMutex
	collectors []Collector
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Register adds a collector to the registry.
func (r *Registry) Register(c Collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectors = append(r.collectors, c)
}

// For returns the collectors that support the given device class.
func (r *Registry) For(class DeviceClass) []Collector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Collector
	for _, c := range r.collectors {
		if c.Supports(class) {
			out = append(out, c)
		}
	}
	return out
}

// All returns every registered collector.
func (r *Registry) All() []Collector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Collector(nil), r.collectors...)
}
