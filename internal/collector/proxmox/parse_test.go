package proxmox

import (
	"testing"

	"github.com/stock3/motzworks/internal/model"
)

func TestBuildHypervisor(t *testing.T) {
	ver := []byte(`{"data":{"version":"8.2.4","release":"8.2","repoid":"abc"}}`)
	nodes := []byte(`{"data":[{"node":"pve1","status":"online","maxcpu":16,"maxmem":68719476736}]}`)
	dev, err := buildHypervisor(ver, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if dev.Type != model.TypeHypervisor {
		t.Errorf("type = %s", dev.Type)
	}
	if dev.Hostname != "pve1" {
		t.Errorf("hostname = %q", dev.Hostname)
	}
	if dev.OS == nil || dev.OS.Version != "8.2.4" || dev.OS.Name != "Proxmox VE" {
		t.Errorf("os = %+v", dev.OS)
	}
	if dev.Hardware == nil || dev.Hardware.CPUCores != 16 || dev.Hardware.RAMBytes != 68719476736 {
		t.Errorf("hardware = %+v", dev.Hardware)
	}
}

func TestBuildVMs(t *testing.T) {
	vms := []byte(`{"data":[
		{"vmid":100,"name":"web01","node":"pve1","type":"qemu","status":"running","maxmem":2147483648,"maxcpu":2},
		{"vmid":200,"name":"ct-dns","node":"pve1","type":"lxc","status":"running","maxmem":536870912,"maxcpu":1},
		{"vmid":300,"name":"","node":"pve1","type":"qemu","status":"stopped"}
	]}`)
	related, err := buildVMs(vms)
	if err != nil {
		t.Fatal(err)
	}
	if len(related) != 2 {
		t.Fatalf("got %d VMs, want 2 (blank-name skipped)", len(related))
	}
	if related[0].Device.Hostname != "web01" || related[0].Kind != "hosts-vm" {
		t.Errorf("related[0] = %+v", related[0])
	}
	if related[0].Device.Type != model.TypeVM {
		t.Errorf("vm type = %s", related[0].Device.Type)
	}
	if related[1].Device.Hardware == nil || related[1].Device.Hardware.RAMBytes != 536870912 {
		t.Errorf("related[1] hw = %+v", related[1].Device.Hardware)
	}
}
