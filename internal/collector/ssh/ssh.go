// Package ssh implements the SSH collector for Unix-like hosts. It runs a small
// set of read-only shell commands and normalizes the output into a model.Device.
package ssh

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	xssh "golang.org/x/crypto/ssh"

	"github.com/stock3/motzworks/internal/collector"
	"github.com/stock3/motzworks/internal/model"
)

// Collector gathers inventory over SSH.
type Collector struct {
	log     *slog.Logger
	Port    int           // default 22 (overridable for tests)
	Timeout time.Duration // dial + command timeout
	MinUID  int           // lowest non-root UID to report as a user
}

// New returns an SSH collector with sensible defaults.
func New(log *slog.Logger) *Collector {
	return &Collector{log: log, Port: 22, Timeout: 10 * time.Second, MinUID: 1000}
}

func (c *Collector) Name() string { return "ssh" }

func (c *Collector) Supports(class collector.DeviceClass) bool {
	return class == collector.ClassLinux || class == collector.ClassMac || class == collector.ClassUnknown
}

// pickCredential returns the first SSH credential in the candidate set.
func pickCredential(creds []collector.Credential) (collector.Credential, bool) {
	for _, cr := range creds {
		if cr.Kind == "ssh-password" || cr.Kind == "ssh-key" {
			return cr, true
		}
	}
	return collector.Credential{}, false
}

// Collect connects over SSH and returns normalized inventory. It returns an
// error only on connection/auth failure; missing individual facts are tolerated.
func (c *Collector) Collect(ctx context.Context, t collector.Target) (collector.Result, error) {
	cred, ok := pickCredential(t.Credentials)
	if !ok {
		return collector.Result{}, errors.New("no ssh credential")
	}

	auth, err := authMethod(cred)
	if err != nil {
		return collector.Result{}, err
	}
	cfg := &xssh.ClientConfig{
		User:            cred.Username,
		Auth:            []xssh.AuthMethod{auth},
		HostKeyCallback: xssh.InsecureIgnoreHostKey(), // scanner context; pinning is a later option
		Timeout:         c.Timeout,
	}

	port := c.Port
	if port == 0 {
		port = 22
	}
	addr := net.JoinHostPort(t.Addr.String(), strconv.Itoa(port))

	d := net.Dialer{Timeout: c.Timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return collector.Result{}, fmt.Errorf("dial: %w", err)
	}
	sc, chans, reqs, err := xssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		return collector.Result{}, fmt.Errorf("ssh handshake: %w", err)
	}
	client := xssh.NewClient(sc, chans, reqs)
	defer client.Close()

	run := func(cmd string) string {
		sess, err := client.NewSession()
		if err != nil {
			return ""
		}
		defer sess.Close()
		out, _ := sess.CombinedOutput(cmd)
		return string(out)
	}

	dev := model.Device{Type: model.TypeLinux, PrimaryIP: t.Addr, Source: "ssh"}
	raw := map[string]any{}

	if h := firstLine(run("hostname -f 2>/dev/null || hostname")); h != "" {
		dev.Hostname = h
	}

	osRel := run("cat /etc/os-release 2>/dev/null")
	raw["os_release"] = osRel
	name, version, family := parseOSRelease(osRel)
	dev.OS = &model.OSInfo{
		Family:  family,
		Name:    name,
		Version: version,
		Build:   firstLine(run("uname -r")),
		Arch:    firstLine(run("uname -m")),
	}
	if dev.OS.Family == "" {
		dev.OS.Family = "linux"
	}

	dev.Interfaces = parseIPLink(run("ip -o link show 2>/dev/null"))
	applyIPAddrs(dev.Interfaces, run("ip -o addr show 2>/dev/null"))

	dev.Software = collectSoftware(run)
	dev.Users = parsePasswd(run("getent passwd 2>/dev/null || cat /etc/passwd"), c.MinUID)
	dev.Hardware = collectHardware(run)

	return collector.Result{Target: t, Device: dev, Raw: raw}, nil
}

// collectSoftware tries dpkg, then rpm, then apk, using the first that returns
// any packages.
func collectSoftware(run func(string) string) []model.Software {
	if out := run(`command -v dpkg-query >/dev/null 2>&1 && dpkg-query -W -f='${Package}\t${Version}\n' 2>/dev/null`); out != "" {
		if sw := parseTabbed(out); len(sw) > 0 {
			return sw
		}
	}
	if out := run(`command -v rpm >/dev/null 2>&1 && rpm -qa --qf '%{NAME}\t%{VERSION}-%{RELEASE}\n' 2>/dev/null`); out != "" {
		if sw := parseTabbed(out); len(sw) > 0 {
			return sw
		}
	}
	if out := run(`command -v apk >/dev/null 2>&1 && apk info -v 2>/dev/null`); out != "" {
		if sw := parseApk(out); len(sw) > 0 {
			return sw
		}
	}
	return nil
}

// collectHardware reads DMI/proc files. Serial/model often require root and may
// be empty in containers or unprivileged sessions; that is tolerated.
func collectHardware(run func(string) string) *model.Hardware {
	hw := &model.Hardware{
		Vendor: firstLine(run("cat /sys/class/dmi/id/sys_vendor 2>/dev/null")),
		Model:  firstLine(run("cat /sys/class/dmi/id/product_name 2>/dev/null")),
		Serial: firstLine(run("cat /sys/class/dmi/id/product_serial 2>/dev/null")),
	}
	hw.CPU, hw.CPUCores = parseCPUInfo(run("cat /proc/cpuinfo 2>/dev/null"))
	hw.RAMBytes = parseMemTotalBytes(run("cat /proc/meminfo 2>/dev/null"))

	if hw.Vendor == "" && hw.Model == "" && hw.Serial == "" && hw.CPU == "" && hw.RAMBytes == 0 {
		return nil
	}
	return hw
}

func authMethod(cred collector.Credential) (xssh.AuthMethod, error) {
	switch cred.Kind {
	case "ssh-key":
		signer, err := xssh.ParsePrivateKey([]byte(cred.Secret))
		if err != nil {
			return nil, fmt.Errorf("parse ssh key: %w", err)
		}
		return xssh.PublicKeys(signer), nil
	case "ssh-password":
		return xssh.Password(cred.Secret), nil
	default:
		return nil, fmt.Errorf("unsupported ssh credential kind %q", cred.Kind)
	}
}

// firstLine returns the first non-empty, trimmed line of s.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}
