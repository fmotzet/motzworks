package wmi

import (
	"encoding/json"
	"net/netip"
	"strconv"
	"strings"

	"github.com/stock3/motzworks/internal/model"
)

// wmiInventory mirrors the JSON emitted by wmi_collect.py.
type wmiInventory struct {
	OS struct {
		Caption        string `json:"Caption"`
		Version        string `json:"Version"`
		BuildNumber    string `json:"BuildNumber"`
		OSArchitecture string `json:"OSArchitecture"`
		CSName         string `json:"CSName"`
	} `json:"os"`
	CS struct {
		Name                string      `json:"Name"`
		Manufacturer        string      `json:"Manufacturer"`
		Model               string      `json:"Model"`
		TotalPhysicalMemory json.Number `json:"TotalPhysicalMemory"`
	} `json:"cs"`
	BIOS struct {
		SerialNumber string `json:"SerialNumber"`
	} `json:"bios"`
	CPU struct {
		Name                      string `json:"Name"`
		NumberOfLogicalProcessors int    `json:"NumberOfLogicalProcessors"`
	} `json:"cpu"`
	Net []struct {
		Description string   `json:"Description"`
		MACAddress  string   `json:"MACAddress"`
		IPAddress   []string `json:"IPAddress"`
	} `json:"net"`
	Users []struct {
		Name     string `json:"Name"`
		FullName string `json:"FullName"`
	} `json:"users"`
	Software []struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		Publisher string `json:"publisher"`
	} `json:"software"`
}

// parseInventory maps the sidecar JSON to a model.Device.
func parseInventory(b []byte) (model.Device, error) {
	var inv wmiInventory
	if err := json.Unmarshal(b, &inv); err != nil {
		return model.Device{}, err
	}

	dev := model.Device{
		Type:     model.TypeWindows,
		Source:   "wmi",
		Hostname: firstNonEmpty(inv.CS.Name, inv.OS.CSName),
		Serial:   strings.TrimSpace(inv.BIOS.SerialNumber),
		OS: &model.OSInfo{
			Family:  "windows",
			Name:    inv.OS.Caption,
			Version: inv.OS.Version,
			Build:   inv.OS.BuildNumber,
			Arch:    inv.OS.OSArchitecture,
		},
		Hardware: &model.Hardware{
			Vendor:   inv.CS.Manufacturer,
			Model:    inv.CS.Model,
			Serial:   strings.TrimSpace(inv.BIOS.SerialNumber),
			CPU:      strings.TrimSpace(inv.CPU.Name),
			CPUCores: inv.CPU.NumberOfLogicalProcessors,
			RAMBytes: toInt64(inv.CS.TotalPhysicalMemory),
		},
	}

	for _, n := range inv.Net {
		ifc := model.Interface{Name: n.Description, MAC: strings.TrimSpace(n.MACAddress)}
		for _, ip := range n.IPAddress {
			if a, err := netip.ParseAddr(strings.TrimSpace(ip)); err == nil && a.Is4() {
				ifc.IP = a
				break
			}
		}
		if ifc.Name == "" && ifc.MAC == "" {
			continue
		}
		dev.Interfaces = append(dev.Interfaces, ifc)
	}

	for _, u := range inv.Users {
		if strings.TrimSpace(u.Name) == "" {
			continue
		}
		dev.Users = append(dev.Users, model.UserAccount{Username: u.Name, FullName: strings.TrimSpace(u.FullName), IsLocal: true})
	}

	for _, s := range inv.Software {
		if strings.TrimSpace(s.Name) == "" {
			continue
		}
		dev.Software = append(dev.Software, model.Software{Name: s.Name, Version: strings.TrimSpace(s.Version), Vendor: strings.TrimSpace(s.Publisher)})
	}

	return dev, nil
}

func toInt64(n json.Number) int64 {
	if n == "" {
		return 0
	}
	v, err := strconv.ParseInt(string(n), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
