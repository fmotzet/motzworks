package fingerprint

import (
	"testing"

	"github.com/stock3/motzworks/internal/collector"
)

func TestClassifyByPorts(t *testing.T) {
	tests := []struct {
		name string
		open []int
		want collector.DeviceClass
	}{
		{"winrm", []int{5985, 445}, collector.ClassWindows},
		{"rdp", []int{3389}, collector.ClassWindows},
		{"smb only", []int{445, 139}, collector.ClassWindows},
		{"ssh", []int{22}, collector.ClassLinux},
		{"ssh+smb prefers windows services? no", []int{22, 445}, collector.ClassLinux},
		{"snmp wins over ssh", []int{22, SNMPPort}, collector.ClassSNMP},
		{"proxmox node (8006 beats ssh)", []int{8006, 22}, collector.ClassHypervisor},
		{"proxmox only", []int{8006}, collector.ClassHypervisor},
		{"printer", []int{9100}, collector.ClassSNMP},
		{"empty", []int{}, collector.ClassUnknown},
		{"http only", []int{80, 443}, collector.ClassUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyByPorts(tt.open); got != tt.want {
				t.Errorf("ClassifyByPorts(%v) = %s, want %s", tt.open, got, tt.want)
			}
		})
	}
}
