// Package model defines the unified asset inventory domain types. Collectors
// produce these; the store persists them. Field-level changes between scans
// drive the change timeline.
package model

import (
	"net/netip"
	"time"
)

// DeviceType classifies an asset.
type DeviceType string

const (
	TypeUnknown    DeviceType = "unknown"
	TypeWindows    DeviceType = "windows"
	TypeLinux      DeviceType = "linux"
	TypeMac        DeviceType = "mac"
	TypePrinter    DeviceType = "printer"
	TypeSwitch     DeviceType = "switch"
	TypeRouter     DeviceType = "router"
	TypeFirewall   DeviceType = "firewall"
	TypeHypervisor DeviceType = "hypervisor"
	TypeVM         DeviceType = "vm"
	TypeUPS        DeviceType = "ups"
)

// Device is the central asset record. Serial, ADGuid, Hostname and interface
// MACs are the identity keys used to merge a device discovered via multiple
// collectors into a single record.
type Device struct {
	ID         string
	Type       DeviceType
	Hostname   string
	PrimaryIP  netip.Addr
	Serial     string
	AssetTag   string
	ADGuid     string
	Source     string // which collector/discovery produced this
	FirstSeen  time.Time
	LastSeen   time.Time
	Interfaces []Interface
	OS         *OSInfo
	Hardware   *Hardware
	Software   []Software
	Users      []UserAccount
}

// Interface is a network interface on a device.
type Interface struct {
	Name      string
	MAC       string
	IP        netip.Addr
	SpeedMbps int64
	VLAN      int
}

// OSInfo holds operating system facts.
type OSInfo struct {
	Family  string // windows, linux, macos, ...
	Name    string
	Version string
	Build   string
	Arch    string
}

// Hardware holds physical/virtual hardware facts.
type Hardware struct {
	Vendor   string
	Model    string
	Serial   string
	CPU      string
	CPUCores int
	RAMBytes int64
}

// Software is one installed application.
type Software struct {
	Name        string
	Version     string
	Vendor      string
	InstallDate string // ISO date as reported; parsing left to the store
}

// UserAccount is a local or directory user discovered on a device.
type UserAccount struct {
	Username  string
	FullName  string
	LastLogon *time.Time
	IsLocal   bool
}
