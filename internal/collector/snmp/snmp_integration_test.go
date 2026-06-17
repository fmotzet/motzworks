package snmp

import (
	"context"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/stock3/motzworks/internal/collector"
)

// TestCollectLive runs the SNMP collector against a real agent. Skipped unless
// MOTZWORKS_SNMP_ADDR is set, e.g.:
//
//	MOTZWORKS_SNMP_ADDR=172.17.0.3 MOTZWORKS_SNMP_COMMUNITY=public \
//	go test ./internal/collector/snmp/ -run Live -v
func TestCollectLive(t *testing.T) {
	addr := os.Getenv("MOTZWORKS_SNMP_ADDR")
	if addr == "" {
		t.Skip("set MOTZWORKS_SNMP_ADDR to run the live SNMP test")
	}
	community := os.Getenv("MOTZWORKS_SNMP_COMMUNITY")
	if community == "" {
		community = "public"
	}

	c := New(nil)
	c.Timeout = 3 * time.Second

	res, err := c.Collect(context.Background(), collector.Target{
		Addr:        netip.MustParseAddr(addr),
		Class:       collector.ClassSNMP,
		Credentials: []collector.Credential{{Kind: "snmp-v2c", Secret: community}},
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if res.Device.OS == nil || res.Device.OS.Name == "" {
		t.Errorf("expected sysDescr-derived OS, got %+v", res.Device.OS)
	}
	t.Logf("collected: host=%s os=%q ifaces=%d", res.Device.Hostname, res.Device.OS.Name, len(res.Device.Interfaces))
}
