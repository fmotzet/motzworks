package snmp

import (
	"testing"

	"github.com/gosnmp/gosnmp"
)

func TestPDUMAC(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want string
	}{
		{"valid", []byte{0x02, 0x42, 0xac, 0x11, 0x00, 0x02}, "02:42:ac:11:00:02"},
		{"all zero", []byte{0, 0, 0, 0, 0, 0}, ""},
		{"wrong length", []byte{0x01, 0x02}, ""},
		{"not bytes", "nope", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pduMAC(gosnmp.SnmpPDU{Value: tt.val}); got != tt.want {
				t.Errorf("pduMAC = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPDUString(t *testing.T) {
	if got := pduString(gosnmp.SnmpPDU{Value: []byte("switch01\x00")}); got != "switch01" {
		t.Errorf("got %q", got)
	}
	if got := pduString(gosnmp.SnmpPDU{Value: "1.3.6.1.4.1.9"}); got != "1.3.6.1.4.1.9" {
		t.Errorf("got %q", got)
	}
	if got := pduString(gosnmp.SnmpPDU{Value: nil}); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestPDUUint(t *testing.T) {
	if got := pduUint(gosnmp.SnmpPDU{Value: uint(1_000_000_000)}); got != 1_000_000_000 {
		t.Errorf("got %d", got)
	}
	if got := pduUint(gosnmp.SnmpPDU{Value: uint32(100)}); got != 100 {
		t.Errorf("got %d", got)
	}
}

func TestOSFamily(t *testing.T) {
	tests := map[string]string{
		"Linux switch01 5.10":           "linux",
		"FortiGate-60F v7.2.5":          "fortios",
		"Cisco IOS Software":            "cisco-ios",
		"Hardware: Intel, Windows":      "windows",
		"Some Unknown Printer Firmware": "",
	}
	for desc, want := range tests {
		if got := osFamily(desc); got != want {
			t.Errorf("osFamily(%q) = %q, want %q", desc, got, want)
		}
	}
}
