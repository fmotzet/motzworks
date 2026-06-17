// Package fingerprint classifies a discovered host into a DeviceClass from the
// ports it has open, so the scan engine can pick the right collector. These are
// best-effort heuristics; collectors confirm (or correct) the class when they
// actually connect.
package fingerprint

import (
	"slices"

	"github.com/stock3/motzworks/internal/collector"
	"github.com/stock3/motzworks/internal/model"
)

// SNMPPort is the conventional UDP SNMP port. Discovery records it on hosts
// that answer an SNMP probe so they classify as SNMP devices here.
const SNMPPort = 161

// ClassifyByPorts returns the most likely DeviceClass for a set of open ports.
func ClassifyByPorts(open []int) collector.DeviceClass {
	has := func(p int) bool { return slices.Contains(open, p) }

	switch {
	// Windows-specific services take precedence.
	case has(5985) || has(5986) || has(3389) || has(135):
		return collector.ClassWindows
	// Anything answering SNMP is treated as network gear / managed device.
	case has(SNMPPort):
		return collector.ClassSNMP
	// Proxmox VE management UI.
	case has(8006):
		return collector.ClassHypervisor
	// SSH without Windows services → Unix-like.
	case has(22):
		return collector.ClassLinux
	// SMB without SSH → most likely Windows.
	case has(445) || has(139):
		return collector.ClassWindows
	// Printer protocols.
	case has(9100) || has(631) || has(515):
		return collector.ClassSNMP
	default:
		return collector.ClassUnknown
	}
}

// ClassToType maps a fingerprinted class to a default device type, used when no
// collector produces a more specific classification.
func ClassToType(c collector.DeviceClass) model.DeviceType {
	switch c {
	case collector.ClassWindows:
		return model.TypeWindows
	case collector.ClassLinux:
		return model.TypeLinux
	case collector.ClassMac:
		return model.TypeMac
	case collector.ClassSNMP:
		return model.TypeSwitch
	case collector.ClassHypervisor:
		return model.TypeHypervisor
	case collector.ClassFirewall:
		return model.TypeFirewall
	default:
		return model.TypeUnknown
	}
}
