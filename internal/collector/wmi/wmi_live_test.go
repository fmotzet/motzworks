package wmi

import (
	"context"
	"net/netip"
	"os"
	"testing"

	"github.com/stock3/motzworks/internal/collector"
)

// TestCollectLive runs the WMI collector against a real Windows host. It needs
// python3 + impacket on PATH and is skipped unless MOTZWORKS_WMI_ADDR is set:
//
//	nix-shell -p 'python3.withPackages(ps:[ps.impacket])' --run '
//	  MOTZWORKS_WMI_ADDR=10.20.30.70 MOTZWORKS_WMI_DOMAIN=AD \
//	  MOTZWORKS_WMI_USER=inventory MOTZWORKS_WMI_PASS=... \
//	  go test ./internal/collector/wmi/ -run Live -v'
func TestCollectLive(t *testing.T) {
	addr := os.Getenv("MOTZWORKS_WMI_ADDR")
	if addr == "" {
		t.Skip("set MOTZWORKS_WMI_ADDR to run the live WMI test")
	}
	c := New(nil)
	cred := collector.Credential{
		Kind:     "wmi",
		Username: os.Getenv("MOTZWORKS_WMI_USER"),
		Secret:   os.Getenv("MOTZWORKS_WMI_PASS"),
		Extra:    map[string]string{"domain": os.Getenv("MOTZWORKS_WMI_DOMAIN")},
	}
	res, err := c.Collect(context.Background(), collector.Target{
		Addr:        netip.MustParseAddr(addr),
		Class:       collector.ClassWindows,
		Credentials: []collector.Credential{cred},
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	d := res.Device
	if d.OS == nil || d.OS.Name == "" {
		t.Errorf("expected OS, got %+v", d.OS)
	}
	t.Logf("collected: host=%s os=%q sw=%d ifaces=%d users=%d",
		d.Hostname, d.OS.Name, len(d.Software), len(d.Interfaces), len(d.Users))
}
