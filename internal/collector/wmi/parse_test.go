package wmi

import (
	"testing"

	"github.com/stock3/motzworks/internal/model"
)

const sampleJSON = `{
  "os": {"Caption":"Microsoft Windows Server 2022 Datacenter","Version":"10.0.20348","BuildNumber":"20348","OSArchitecture":"64-Bit","CSName":"BGDC1PO-ADTS01"},
  "cs": {"Name":"BGDC1PO-ADTS01","Domain":"ad.boerse-go.de","Manufacturer":"QEMU","Model":"Standard PC","TotalPhysicalMemory":17179308032},
  "bios": {"SerialNumber":"VMW-123"},
  "cpu": {"Name":"Intel Xeon","NumberOfLogicalProcessors":4},
  "net": [
    {"Description":"Intel NIC","MACAddress":"AA:BB:CC:DD:EE:01","IPAddress":["fe80::1","10.20.30.70"]}
  ],
  "users": [
    {"Name":"Administrator","FullName":"Admin"},
    {"Name":"","FullName":"skip me"}
  ],
  "software": [
    {"name":"7-Zip","version":"23.01","publisher":"Igor Pavlov"},
    {"name":"","version":"1.0","publisher":""}
  ]
}`

func TestParseInventory(t *testing.T) {
	dev, err := parseInventory([]byte(sampleJSON))
	if err != nil {
		t.Fatal(err)
	}
	if dev.Type != model.TypeWindows {
		t.Errorf("type = %s", dev.Type)
	}
	if dev.Hostname != "BGDC1PO-ADTS01" {
		t.Errorf("hostname = %q", dev.Hostname)
	}
	if dev.Serial != "VMW-123" {
		t.Errorf("serial = %q", dev.Serial)
	}
	if dev.OS.Name != "Microsoft Windows Server 2022 Datacenter" || dev.OS.Build != "20348" {
		t.Errorf("os = %+v", dev.OS)
	}
	if dev.Hardware.Vendor != "QEMU" || dev.Hardware.CPUCores != 4 || dev.Hardware.RAMBytes != 17179308032 {
		t.Errorf("hw = %+v", dev.Hardware)
	}
	if len(dev.Interfaces) != 1 || dev.Interfaces[0].IP.String() != "10.20.30.70" {
		t.Fatalf("interfaces = %+v (should pick the IPv4)", dev.Interfaces)
	}
	if len(dev.Users) != 1 || dev.Users[0].Username != "Administrator" {
		t.Errorf("users = %+v (blank name filtered)", dev.Users)
	}
	if len(dev.Software) != 1 || dev.Software[0].Name != "7-Zip" || dev.Software[0].Vendor != "Igor Pavlov" {
		t.Errorf("software = %+v (blank name filtered)", dev.Software)
	}
}

func TestSplitDomainUser(t *testing.T) {
	tests := []struct{ in, fb, wantUser, wantDom string }{
		{`AD\inventory`, "", "inventory", "AD"},
		{"inventory@ad.boerse-go.de", "", "inventory", "ad.boerse-go.de"},
		{"inventory", "AD", "inventory", "AD"},
		{"inventory", "", "inventory", ""},
	}
	for _, tt := range tests {
		u, d := splitDomainUser(tt.in, tt.fb)
		if u != tt.wantUser || d != tt.wantDom {
			t.Errorf("splitDomainUser(%q,%q) = (%q,%q), want (%q,%q)", tt.in, tt.fb, u, d, tt.wantUser, tt.wantDom)
		}
	}
}
