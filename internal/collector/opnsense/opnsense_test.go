package opnsense

import (
	"testing"

	"github.com/stock3/motzworks/internal/model"
)

func TestParseFirmwareNested(t *testing.T) {
	raw := []byte(`{"status":"none","product":{"product_name":"OPNsense","product_version":"24.7.3"}}`)
	dev, err := parseFirmware(raw)
	if err != nil {
		t.Fatal(err)
	}
	if dev.Type != model.TypeFirewall {
		t.Errorf("type = %s", dev.Type)
	}
	if dev.OS == nil || dev.OS.Version != "24.7.3" || dev.OS.Family != "opnsense" {
		t.Errorf("os = %+v", dev.OS)
	}
}

func TestParseFirmwareTopLevel(t *testing.T) {
	raw := []byte(`{"product_name":"OPNsense","product_version":"24.1"}`)
	dev, err := parseFirmware(raw)
	if err != nil {
		t.Fatal(err)
	}
	if dev.OS.Version != "24.1" {
		t.Errorf("version = %q", dev.OS.Version)
	}
}

func TestParseFirmwareNotOPNsense(t *testing.T) {
	// A non-OPNsense JSON 200 must be rejected (self-identification).
	raw := []byte(`{"product_name":"pfSense","product_version":"2.7"}`)
	if _, err := parseFirmware(raw); err == nil {
		t.Error("expected rejection for non-OPNsense payload")
	}
}
