package ssh

import (
	"context"
	"net/netip"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stock3/motzworks/internal/collector"
)

// TestCollectLive runs the SSH collector against a real host. It is skipped
// unless MOTZWORKS_SSH_ADDR (and credentials) are set, e.g.:
//
//	MOTZWORKS_SSH_ADDR=172.17.0.2 MOTZWORKS_SSH_USER=test \
//	MOTZWORKS_SSH_PASS=testpass go test ./internal/collector/ssh/ -run Live -v
func TestCollectLive(t *testing.T) {
	addr := os.Getenv("MOTZWORKS_SSH_ADDR")
	user := os.Getenv("MOTZWORKS_SSH_USER")
	if addr == "" || user == "" {
		t.Skip("set MOTZWORKS_SSH_ADDR / MOTZWORKS_SSH_USER to run the live SSH test")
	}

	c := New(nil)
	if p := os.Getenv("MOTZWORKS_SSH_PORT"); p != "" {
		c.Port, _ = strconv.Atoi(p)
	}
	c.Timeout = 5 * time.Second

	cred := collector.Credential{Kind: "ssh-password", Username: user, Secret: os.Getenv("MOTZWORKS_SSH_PASS")}
	if key := os.Getenv("MOTZWORKS_SSH_KEY"); key != "" {
		b, err := os.ReadFile(key)
		if err != nil {
			t.Fatal(err)
		}
		cred = collector.Credential{Kind: "ssh-key", Username: user, Secret: string(b)}
	}

	res, err := c.Collect(context.Background(), collector.Target{
		Addr:        netip.MustParseAddr(addr),
		Class:       collector.ClassLinux,
		Credentials: []collector.Credential{cred},
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	d := res.Device
	if d.OS == nil || d.OS.Name == "" {
		t.Errorf("expected OS info, got %+v", d.OS)
	}
	if len(d.Software) == 0 {
		t.Error("expected at least one software package")
	}
	for _, ifc := range d.Interfaces {
		if ifc.Name == "" || ifc.MAC == "" {
			t.Errorf("interface missing name/mac: %+v", ifc)
		}
	}
	t.Logf("collected: host=%s os=%q sw=%d ifaces=%d users=%d",
		d.Hostname, d.OS.Name, len(d.Software), len(d.Interfaces), len(d.Users))
}
