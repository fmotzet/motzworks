package winrm

import (
	"bytes"
	"encoding/json"
	"net/netip"
	"strconv"
	"strings"

	"github.com/stock3/motzworks/internal/model"
)

// The inventory PowerShell script emits a single JSON object. These types
// mirror it. Array fields are json.RawMessage because Windows PowerShell 5.1
// collapses single-element arrays to a bare object — toObjects normalizes that.

type inventory struct {
	OS       cimOS           `json:"OS"`
	Computer cimComputer     `json:"Computer"`
	BIOS     cimBIOS         `json:"BIOS"`
	CPU      cimCPU          `json:"CPU"`
	Network  json.RawMessage `json:"Network"`
	Software json.RawMessage `json:"Software"`
	Users    json.RawMessage `json:"Users"`
}

type cimOS struct {
	Caption        string `json:"Caption"`
	Version        string `json:"Version"`
	BuildNumber    string `json:"BuildNumber"`
	OSArchitecture string `json:"OSArchitecture"`
}

type cimComputer struct {
	Name                string      `json:"Name"`
	Domain              string      `json:"Domain"`
	Manufacturer        string      `json:"Manufacturer"`
	Model               string      `json:"Model"`
	TotalPhysicalMemory json.Number `json:"TotalPhysicalMemory"`
}

type cimBIOS struct {
	SerialNumber string `json:"SerialNumber"`
}

type cimCPU struct {
	Name                      string `json:"Name"`
	NumberOfLogicalProcessors int    `json:"NumberOfLogicalProcessors"`
}

type cimNet struct {
	Description string     `json:"Description"`
	MACAddress  string     `json:"MACAddress"`
	IPAddress   stringList `json:"IPAddress"`
}

type cimSoftware struct {
	DisplayName    string `json:"DisplayName"`
	DisplayVersion string `json:"DisplayVersion"`
	Publisher      string `json:"Publisher"`
	InstallDate    string `json:"InstallDate"`
}

type cimUser struct {
	Name     string `json:"Name"`
	FullName string `json:"FullName"`
}

// stringList unmarshals a JSON value that may be a single string or an array of
// strings (CIM IPAddress is multivalued but collapses when single).
type stringList []string

func (s *stringList) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '[' {
		var arr []string
		if err := json.Unmarshal(b, &arr); err != nil {
			return err
		}
		*s = arr
		return nil
	}
	var single string
	if err := json.Unmarshal(b, &single); err != nil {
		return err
	}
	*s = []string{single}
	return nil
}

// toObjects normalizes a PowerShell array field into a slice of element JSON,
// accepting either a real array or a single collapsed object.
func toObjects(raw json.RawMessage) []json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if trimmed[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return nil
		}
		return arr
	}
	return []json.RawMessage{trimmed}
}

// parseInventory converts the inventory JSON into a model.Device.
func parseInventory(b []byte) (model.Device, error) {
	var inv inventory
	if err := json.Unmarshal(b, &inv); err != nil {
		return model.Device{}, err
	}

	dev := model.Device{
		Type:     model.TypeWindows,
		Source:   "winrm",
		Hostname: inv.Computer.Name,
		Serial:   strings.TrimSpace(inv.BIOS.SerialNumber),
		OS: &model.OSInfo{
			Family:  "windows",
			Name:    inv.OS.Caption,
			Version: inv.OS.Version,
			Build:   inv.OS.BuildNumber,
			Arch:    inv.OS.OSArchitecture,
		},
		Hardware: &model.Hardware{
			Vendor:   inv.Computer.Manufacturer,
			Model:    inv.Computer.Model,
			Serial:   strings.TrimSpace(inv.BIOS.SerialNumber),
			CPU:      strings.TrimSpace(inv.CPU.Name),
			CPUCores: inv.CPU.NumberOfLogicalProcessors,
			RAMBytes: parseInt64(inv.Computer.TotalPhysicalMemory),
		},
	}

	for _, raw := range toObjects(inv.Network) {
		var n cimNet
		if err := json.Unmarshal(raw, &n); err != nil {
			continue
		}
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

	for _, raw := range toObjects(inv.Software) {
		var s cimSoftware
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		name := strings.TrimSpace(s.DisplayName)
		if name == "" {
			continue
		}
		dev.Software = append(dev.Software, model.Software{
			Name:        name,
			Version:     strings.TrimSpace(s.DisplayVersion),
			Vendor:      strings.TrimSpace(s.Publisher),
			InstallDate: strings.TrimSpace(s.InstallDate),
		})
	}

	for _, raw := range toObjects(inv.Users) {
		var u cimUser
		if err := json.Unmarshal(raw, &u); err != nil {
			continue
		}
		if strings.TrimSpace(u.Name) == "" {
			continue
		}
		dev.Users = append(dev.Users, model.UserAccount{
			Username: u.Name,
			FullName: strings.TrimSpace(u.FullName),
			IsLocal:  true,
		})
	}

	return dev, nil
}

func parseInt64(n json.Number) int64 {
	if n == "" {
		return 0
	}
	v, err := strconv.ParseInt(string(n), 10, 64)
	if err != nil {
		return 0
	}
	return v
}
