// Package wmi implements the agentless Windows collector using WMI over DCOM
// (NTLM, port 135 + dynamic RPC) — the transport Spiceworks Inventory used,
// which works against hosts whose WinRM listener is Kerberos-only. The DCOM/WMI
// dance is delegated to a small embedded impacket (Python) sidecar; pure-Go
// DCOM activation against hardened Windows requires hand-marshaling activation
// blobs, which isn't worth the complexity. This is the primary Windows
// collector (WinRM is the secondary).
package wmi

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/stock3/motzworks/internal/collector"
)

//go:embed wmi_collect.py
var script []byte

// Collector gathers Windows inventory over WMI/DCOM via the impacket sidecar.
type Collector struct {
	log     *slog.Logger
	Python  string // interpreter (default "python3", override via MOTZWORKS_PYTHON)
	Timeout time.Duration
}

// New returns a WMI collector with sensible defaults.
func New(log *slog.Logger) *Collector {
	py := os.Getenv("MOTZWORKS_PYTHON")
	if py == "" {
		py = "python3"
	}
	return &Collector{log: log, Python: py, Timeout: 90 * time.Second}
}

func (c *Collector) Name() string { return "wmi" }

func (c *Collector) Supports(class collector.DeviceClass) bool {
	return class == collector.ClassWindows
}

// pickCredential returns the first WMI credential.
func pickCredential(creds []collector.Credential) (collector.Credential, bool) {
	for _, cr := range creds {
		if cr.Kind == "wmi" {
			return cr, true
		}
	}
	return collector.Credential{}, false
}

// Collect runs the WMI inventory sidecar against the target and normalizes the
// result.
func (c *Collector) Collect(ctx context.Context, t collector.Target) (collector.Result, error) {
	cred, ok := pickCredential(t.Credentials)
	if !ok {
		return collector.Result{}, collector.ErrNoCredential
	}

	user, domain := splitDomainUser(cred.Username, cred.Extra["domain"])

	runCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	// Script on stdin, connection params via env (never argv, so the password
	// never lands in the process list).
	cmd := exec.CommandContext(runCtx, c.Python, "-")
	cmd.Stdin = bytes.NewReader(script)
	cmd.Env = append(os.Environ(),
		"WMI_ADDR="+t.Addr.String(),
		"WMI_USER="+user,
		"WMI_PASS="+cred.Secret,
		"WMI_DOMAIN="+domain,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return collector.Result{}, fmt.Errorf("wmi sidecar exit %d: %s", exit.ExitCode(), oneLine(stderr.String()))
		}
		return collector.Result{}, fmt.Errorf("wmi sidecar: %w (is %s + impacket installed?)", err, c.Python)
	}

	dev, err := parseInventory(stdout.Bytes())
	if err != nil {
		return collector.Result{}, fmt.Errorf("parse wmi inventory: %w", err)
	}
	dev.PrimaryIP = t.Addr

	return collector.Result{Target: t, Device: dev, Raw: map[string]any{"wmi": stdout.String()}}, nil
}

// splitDomainUser resolves the NTLM (domain, user) from a username that may be
// "DOMAIN\\user", "user@domain", or plain (with domain supplied separately).
func splitDomainUser(username, fallbackDomain string) (user, domain string) {
	username = strings.TrimSpace(username)
	if i := strings.IndexAny(username, `\/`); i >= 0 {
		return username[i+1:], username[:i]
	}
	if i := strings.IndexByte(username, '@'); i >= 0 {
		return username[:i], username[i+1:]
	}
	return username, strings.TrimSpace(fallbackDomain)
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
