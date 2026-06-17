package winrm

import "testing"

// Full payload with multi-element arrays (PowerShell 7 / multiple items).
const sampleJSON = `{
  "OS": {"Caption":"Microsoft Windows Server 2022 Standard","Version":"10.0.20348","BuildNumber":"20348","OSArchitecture":"64-bit"},
  "Computer": {"Name":"DC01","Domain":"corp.example.com","Manufacturer":"Dell Inc.","Model":"PowerEdge R740","TotalPhysicalMemory":34359738368},
  "BIOS": {"SerialNumber":"ABC1234"},
  "CPU": {"Name":"Intel(R) Xeon(R) Silver 4210","NumberOfLogicalProcessors":20},
  "Network": [
    {"Description":"Intel I350","MACAddress":"AA:BB:CC:DD:EE:01","IPAddress":["10.0.0.10","fe80::1"]},
    {"Description":"Intel I350 #2","MACAddress":"AA:BB:CC:DD:EE:02","IPAddress":"10.0.0.11"}
  ],
  "Software": [
    {"DisplayName":"7-Zip 23.01","DisplayVersion":"23.01","Publisher":"Igor Pavlov","InstallDate":"20231015"},
    {"DisplayName":"","DisplayVersion":"1.0"},
    {"DisplayName":"Google Chrome","DisplayVersion":"120.0","Publisher":"Google LLC"}
  ],
  "Users": [
    {"Name":"Administrator","FullName":"Administrator"},
    {"Name":"svc_scan","FullName":"Scanner Service"}
  ]
}`

func TestParseInventoryFull(t *testing.T) {
	dev, err := parseInventory([]byte(sampleJSON))
	if err != nil {
		t.Fatalf("parseInventory: %v", err)
	}
	if dev.Hostname != "DC01" {
		t.Errorf("hostname = %q", dev.Hostname)
	}
	if dev.Serial != "ABC1234" {
		t.Errorf("serial = %q", dev.Serial)
	}
	if dev.OS.Name != "Microsoft Windows Server 2022 Standard" || dev.OS.Build != "20348" {
		t.Errorf("os = %+v", dev.OS)
	}
	if dev.Hardware.Model != "PowerEdge R740" || dev.Hardware.CPUCores != 20 {
		t.Errorf("hardware = %+v", dev.Hardware)
	}
	if dev.Hardware.RAMBytes != 34359738368 {
		t.Errorf("ram = %d", dev.Hardware.RAMBytes)
	}

	if len(dev.Interfaces) != 2 {
		t.Fatalf("interfaces = %d, want 2", len(dev.Interfaces))
	}
	// first interface: IPv4 picked over IPv6
	if dev.Interfaces[0].IP.String() != "10.0.0.10" {
		t.Errorf("iface0 ip = %s, want 10.0.0.10", dev.Interfaces[0].IP)
	}
	// second interface: IPAddress was a bare string
	if dev.Interfaces[1].IP.String() != "10.0.0.11" {
		t.Errorf("iface1 ip = %s, want 10.0.0.11", dev.Interfaces[1].IP)
	}

	// blank-named software entry filtered out
	if len(dev.Software) != 2 {
		t.Fatalf("software = %d, want 2", len(dev.Software))
	}
	if dev.Software[0].Name != "7-Zip 23.01" || dev.Software[0].InstallDate != "20231015" {
		t.Errorf("software[0] = %+v", dev.Software[0])
	}

	if len(dev.Users) != 2 || dev.Users[1].Username != "svc_scan" {
		t.Errorf("users = %+v", dev.Users)
	}
}

// PowerShell 5.1 collapses single-element arrays to bare objects. The parser
// must accept Network/Software/Users as a single object, not just an array.
const collapsedJSON = `{
  "OS": {"Caption":"Windows 10 Pro","Version":"10.0.19045","BuildNumber":"19045","OSArchitecture":"64-bit"},
  "Computer": {"Name":"WKS-7","Manufacturer":"LENOVO","Model":"ThinkPad","TotalPhysicalMemory":17179869184},
  "BIOS": {"SerialNumber":"PF0XYZ"},
  "CPU": {"Name":"Intel Core i7","NumberOfLogicalProcessors":8},
  "Network": {"Description":"Realtek","MACAddress":"11:22:33:44:55:66","IPAddress":"192.168.1.50"},
  "Software": {"DisplayName":"Mozilla Firefox","DisplayVersion":"121.0","Publisher":"Mozilla"},
  "Users": {"Name":"alice","FullName":"Alice"}
}`

func TestParseInventoryCollapsed(t *testing.T) {
	dev, err := parseInventory([]byte(collapsedJSON))
	if err != nil {
		t.Fatalf("parseInventory: %v", err)
	}
	if len(dev.Interfaces) != 1 || dev.Interfaces[0].MAC != "11:22:33:44:55:66" {
		t.Errorf("interfaces = %+v", dev.Interfaces)
	}
	if dev.Interfaces[0].IP.String() != "192.168.1.50" {
		t.Errorf("ip = %s", dev.Interfaces[0].IP)
	}
	if len(dev.Software) != 1 || dev.Software[0].Name != "Mozilla Firefox" {
		t.Errorf("software = %+v", dev.Software)
	}
	if len(dev.Users) != 1 || dev.Users[0].Username != "alice" {
		t.Errorf("users = %+v", dev.Users)
	}
}

func TestParseInventoryEmptyArrays(t *testing.T) {
	// Empty arrays / nulls should not panic or add rows.
	const j = `{"OS":{"Caption":"X"},"Computer":{"Name":"H"},"BIOS":{},"CPU":{},"Network":[],"Software":null,"Users":[]}`
	dev, err := parseInventory([]byte(j))
	if err != nil {
		t.Fatalf("parseInventory: %v", err)
	}
	if len(dev.Interfaces) != 0 || len(dev.Software) != 0 || len(dev.Users) != 0 {
		t.Errorf("expected no child rows, got %+v", dev)
	}
}
