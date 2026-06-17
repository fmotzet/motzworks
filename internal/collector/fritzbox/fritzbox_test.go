package fritzbox

import (
	"errors"
	"testing"

	"github.com/stock3/motzworks/internal/collector"
	"github.com/stock3/motzworks/internal/model"
)

// sampleDesc is a trimmed real tr64desc.xml from a FRITZ!Box 7682.
const sampleDesc = `<?xml version="1.0"?>
<root xmlns="urn:dslforum-org:device-1-0">
<specVersion><major>1</major><minor>0</minor></specVersion>
<systemVersion>
<HW>286</HW><Major>286</Major><Minor>8</Minor><Patch>3</Patch>
<Buildnumber>118255</Buildnumber>
<Display>286.08.03</Display>
</systemVersion>
<device>
<deviceType>urn:dslforum-org:device:InternetGatewayDevice:1</deviceType>
<friendlyName>FRITZ!Box 7682</friendlyName>
<manufacturer>AVM</manufacturer>
<modelDescription>FRITZ!Box 7682</modelDescription>
<modelName>FRITZ!Box 7682</modelName>
<modelNumber>7682 - avm</modelNumber>
<UDN>uuid:739f2409-bccb-40e7-8e6c-08B657FAA2AF</UDN>
<serialNumber>08:B6:57:FA:A2:AF</serialNumber>
</device>
</root>`

func TestParseDescFritzbox(t *testing.T) {
	dev, err := parseDesc([]byte(sampleDesc))
	if err != nil {
		t.Fatal(err)
	}
	if dev.Type != model.TypeRouter {
		t.Errorf("type = %s, want router", dev.Type)
	}
	if dev.Source != "fritzbox" {
		t.Errorf("source = %s", dev.Source)
	}
	if dev.Hardware == nil || dev.Hardware.Vendor != "AVM" || dev.Hardware.Model != "FRITZ!Box 7682" {
		t.Errorf("hardware = %+v", dev.Hardware)
	}
	if dev.Hardware.Serial != "08:B6:57:FA:A2:AF" {
		t.Errorf("serial = %q", dev.Hardware.Serial)
	}
	if dev.OS == nil || dev.OS.Family != "fritzos" || dev.OS.Version != "286.08.03" {
		t.Errorf("os = %+v", dev.OS)
	}
}

func TestParseDescNotAVM(t *testing.T) {
	// A non-AVM device description must be rejected as not-applicable so the
	// host stays discovered-only rather than being flagged as a failure.
	raw := []byte(`<?xml version="1.0"?>
<root xmlns="urn:dslforum-org:device-1-0">
<device><manufacturer>Acme</manufacturer><modelName>Router 1</modelName></device>
</root>`)
	_, err := parseDesc(raw)
	if !errors.Is(err, collector.ErrNoCredential) {
		t.Errorf("err = %v, want ErrNoCredential", err)
	}
}

func TestParseDescGarbage(t *testing.T) {
	if _, err := parseDesc([]byte("not xml at all")); !errors.Is(err, collector.ErrNoCredential) {
		t.Errorf("err = %v, want ErrNoCredential", err)
	}
}
