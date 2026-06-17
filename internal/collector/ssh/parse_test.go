package ssh

import "testing"

func TestParseOSRelease(t *testing.T) {
	content := `NAME="Ubuntu"
VERSION_ID="22.04"
PRETTY_NAME="Ubuntu 22.04.3 LTS"
ID=ubuntu`
	name, version, id := parseOSRelease(content)
	if name != "Ubuntu 22.04.3 LTS" {
		t.Errorf("name = %q", name)
	}
	if version != "22.04" {
		t.Errorf("version = %q", version)
	}
	if id != "ubuntu" {
		t.Errorf("id = %q", id)
	}
}

func TestParseTabbed(t *testing.T) {
	out := "nginx\t1.24.0-1\nopenssl\t3.0.2\n\nbad-line-no-tab\n"
	sw := parseTabbed(out)
	if len(sw) != 3 {
		t.Fatalf("got %d packages, want 3: %+v", len(sw), sw)
	}
	if sw[0].Name != "nginx" || sw[0].Version != "1.24.0-1" {
		t.Errorf("sw[0] = %+v", sw[0])
	}
	if sw[2].Name != "bad-line-no-tab" || sw[2].Version != "" {
		t.Errorf("sw[2] = %+v", sw[2])
	}
}

func TestParseApk(t *testing.T) {
	out := "musl-1.2.4-r2\nca-certificates-bundle-20240705-r0\nzlib-1.3.1-r0\n"
	sw := parseApk(out)
	if len(sw) != 3 {
		t.Fatalf("got %d, want 3", len(sw))
	}
	if sw[1].Name != "ca-certificates-bundle" || sw[1].Version != "20240705-r0" {
		t.Errorf("sw[1] = %+v (name should keep internal dashes)", sw[1])
	}
}

func TestParseIPLinkAndAddr(t *testing.T) {
	link := `1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN mode DEFAULT group default qlen 1000\    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc fq_codel state UP mode DEFAULT group default qlen 1000\    link/ether 02:42:ac:11:00:02 brd ff:ff:ff:ff:ff:ff`
	ifaces := parseIPLink(link)
	if len(ifaces) != 1 {
		t.Fatalf("got %d ifaces, want 1 (lo excluded): %+v", len(ifaces), ifaces)
	}
	if ifaces[0].Name != "eth0" || ifaces[0].MAC != "02:42:ac:11:00:02" {
		t.Fatalf("iface = %+v", ifaces[0])
	}

	addr := `2: eth0    inet 172.17.0.2/16 brd 172.17.255.255 scope global eth0\       valid_lft forever preferred_lft forever`
	applyIPAddrs(ifaces, addr)
	if ifaces[0].IP.String() != "172.17.0.2" {
		t.Errorf("ip = %s, want 172.17.0.2", ifaces[0].IP)
	}
}

func TestParsePasswd(t *testing.T) {
	out := `root:x:0:0:root:/root:/bin/bash
daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin
alice:x:1000:1000:Alice Smith,,,:/home/alice:/bin/bash`
	users := parsePasswd(out, 1000)
	if len(users) != 2 {
		t.Fatalf("got %d users, want 2 (root + alice): %+v", len(users), users)
	}
	var alice *struct{ ok bool }
	for _, u := range users {
		if u.Username == "alice" {
			alice = &struct{ ok bool }{true}
			if u.FullName != "Alice Smith" {
				t.Errorf("alice fullname = %q", u.FullName)
			}
		}
	}
	if alice == nil {
		t.Error("alice not found")
	}
}

func TestParseMemTotalBytes(t *testing.T) {
	out := "MemTotal:       16331828 kB\nMemFree:         123 kB\n"
	if got := parseMemTotalBytes(out); got != 16331828*1024 {
		t.Errorf("got %d", got)
	}
}

func TestParseCPUInfo(t *testing.T) {
	out := `processor	: 0
model name	: Intel(R) Xeon(R) CPU E5-2670 0 @ 2.60GHz
processor	: 1
model name	: Intel(R) Xeon(R) CPU E5-2670 0 @ 2.60GHz`
	model, cores := parseCPUInfo(out)
	if cores != 2 {
		t.Errorf("cores = %d, want 2", cores)
	}
	if model == "" {
		t.Error("model empty")
	}
}
