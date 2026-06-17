package fortigate

import (
	"testing"

	"github.com/stock3/motzworks/internal/model"
)

func TestParseStatusTopLevel(t *testing.T) {
	raw := []byte(`{"serial":"FGT60FTK1234","version":"v7.2.5","build":1517,"hostname":"fw-edge","model":"FortiGate-60F","status":"success"}`)
	dev, err := parseStatus(raw)
	if err != nil {
		t.Fatal(err)
	}
	if dev.Type != model.TypeFirewall {
		t.Errorf("type = %s", dev.Type)
	}
	if dev.Hostname != "fw-edge" || dev.Serial != "FGT60FTK1234" {
		t.Errorf("dev = %+v", dev)
	}
	if dev.OS == nil || dev.OS.Version != "v7.2.5" || dev.OS.Family != "fortios" {
		t.Errorf("os = %+v", dev.OS)
	}
	if dev.Hardware == nil || dev.Hardware.Model != "FortiGate-60F" || dev.Hardware.Vendor != "Fortinet" {
		t.Errorf("hw = %+v", dev.Hardware)
	}
}

func TestParseStatusResultsFallback(t *testing.T) {
	raw := []byte(`{"serial":"FG100F0000","version":"v7.4.1","results":{"hostname":"core-fw","model":"FortiGate-100F"}}`)
	dev, err := parseStatus(raw)
	if err != nil {
		t.Fatal(err)
	}
	if dev.Hostname != "core-fw" || dev.Hardware.Model != "FortiGate-100F" {
		t.Errorf("dev = %+v hw = %+v", dev, dev.Hardware)
	}
}

func TestParseStatusNotForti(t *testing.T) {
	raw := []byte(`{"foo":"bar"}`)
	if _, err := parseStatus(raw); err == nil {
		t.Error("expected rejection for non-FortiGate payload")
	}
}
