package discovery

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestExpand(t *testing.T) {
	tests := []struct {
		name  string
		specs []string
		want  int
		first string
		last  string
	}{
		{"single", []string{"10.0.0.5"}, 1, "10.0.0.5", "10.0.0.5"},
		{"cidr /30", []string{"192.168.1.0/30"}, 4, "192.168.1.0", "192.168.1.3"},
		{"range", []string{"10.0.0.1-10.0.0.4"}, 4, "10.0.0.1", "10.0.0.4"},
		{"dedup", []string{"10.0.0.1", "10.0.0.1", "10.0.0.0/31"}, 2, "10.0.0.1", "10.0.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Expand(tt.specs)
			if err != nil {
				t.Fatalf("Expand: %v", err)
			}
			if len(got) != tt.want {
				t.Fatalf("got %d addrs, want %d (%v)", len(got), tt.want, got)
			}
			if got[0].String() != tt.first {
				t.Errorf("first = %s, want %s", got[0], tt.first)
			}
			if got[len(got)-1].String() != tt.last {
				t.Errorf("last = %s, want %s", got[len(got)-1], tt.last)
			}
		})
	}
}

func TestExpandErrors(t *testing.T) {
	for _, spec := range []string{"not-an-ip", "10.0.0.0/99", "10.0.0.5-10.0.0.1"} {
		if _, err := Expand([]string{spec}); err == nil {
			t.Errorf("Expand(%q) expected error", spec)
		}
	}
}

func TestDiscover(t *testing.T) {
	// Listen on a random localhost port and confirm discovery finds it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	addr := netip.MustParseAddr("127.0.0.1")

	live := Discover(context.Background(), []netip.Addr{addr},
		Options{TCPPorts: []int{port}, Timeout: time.Second, Concurrency: 4})

	if len(live) != 1 {
		t.Fatalf("got %d live hosts, want 1", len(live))
	}
	if !live[0].HasPort(port) {
		t.Errorf("expected open port %d, got %v", port, live[0].OpenPorts)
	}
}

func TestDiscoverNoneAlive(t *testing.T) {
	// Port 1 on a documentation-range address should never connect quickly.
	addr := netip.MustParseAddr("192.0.2.1")
	live := Discover(context.Background(), []netip.Addr{addr},
		Options{TCPPorts: []int{1}, Timeout: 200 * time.Millisecond, Concurrency: 1})
	if len(live) != 0 {
		t.Fatalf("expected no live hosts, got %v", live)
	}
}
