package proxmox

import (
	"encoding/json"

	"github.com/stock3/motzworks/internal/collector"
	"github.com/stock3/motzworks/internal/model"
)

// Proxmox VE API response shapes (subset). All responses wrap payload in "data".

type versionResp struct {
	Data struct {
		Version string `json:"version"`
		Release string `json:"release"`
		RepoID  string `json:"repoid"`
	} `json:"data"`
}

type nodesResp struct {
	Data []nodeInfo `json:"data"`
}

type nodeInfo struct {
	Node   string `json:"node"`
	Status string `json:"status"`
	MaxCPU int    `json:"maxcpu"`
	MaxMem int64  `json:"maxmem"`
}

type vmResp struct {
	Data []vmInfo `json:"data"`
}

type vmInfo struct {
	VMID   int    `json:"vmid"`
	Name   string `json:"name"`
	Node   string `json:"node"`
	Type   string `json:"type"` // qemu | lxc
	Status string `json:"status"`
	MaxMem int64  `json:"maxmem"`
	MaxCPU int    `json:"maxcpu"`
}

// buildHypervisor maps the version + node info to the hypervisor device.
func buildHypervisor(verRaw, nodesRaw []byte) (model.Device, error) {
	var ver versionResp
	if err := json.Unmarshal(verRaw, &ver); err != nil {
		return model.Device{}, err
	}
	dev := model.Device{
		Type:   model.TypeHypervisor,
		Source: "proxmox",
		OS: &model.OSInfo{
			Family:  "proxmox",
			Name:    "Proxmox VE",
			Version: ver.Data.Version,
		},
	}

	var nodes nodesResp
	if err := json.Unmarshal(nodesRaw, &nodes); err == nil && len(nodes.Data) > 0 {
		n := nodes.Data[0]
		dev.Hostname = n.Node
		dev.Hardware = &model.Hardware{
			Vendor:   "Proxmox",
			CPUCores: n.MaxCPU,
			RAMBytes: n.MaxMem,
		}
	}
	return dev, nil
}

// buildVMs maps cluster VM resources to related VM devices.
func buildVMs(vmsRaw []byte) ([]collector.Related, error) {
	var vms vmResp
	if err := json.Unmarshal(vmsRaw, &vms); err != nil {
		return nil, err
	}
	var related []collector.Related
	for _, vm := range vms.Data {
		if vm.Name == "" {
			continue
		}
		d := model.Device{
			Type:     model.TypeVM,
			Hostname: vm.Name,
			Source:   "proxmox",
		}
		if vm.MaxCPU > 0 || vm.MaxMem > 0 {
			d.Hardware = &model.Hardware{CPUCores: vm.MaxCPU, RAMBytes: vm.MaxMem}
		}
		related = append(related, collector.Related{Device: d, Kind: "hosts-vm"})
	}
	return related, nil
}
