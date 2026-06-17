// Package winrm implements the Windows collector. It runs a single PowerShell
// script over WinRM that emits a JSON inventory document (OS, hardware, network,
// installed software from the registry, and local users), then normalizes it
// into a model.Device.
package winrm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/masterzen/winrm"

	"github.com/stock3/motzworks/internal/collector"
)

// Collector gathers inventory over WinRM.
type Collector struct {
	log      *slog.Logger
	Port     int
	UseHTTPS bool
	Insecure bool // skip TLS verification (self-signed WinRM certs)
	Timeout  time.Duration
}

// New returns a WinRM collector with sensible defaults (HTTP/5985).
func New(log *slog.Logger) *Collector {
	return &Collector{log: log, Port: 5985, Timeout: 30 * time.Second}
}

func (c *Collector) Name() string { return "winrm" }

func (c *Collector) Supports(class collector.DeviceClass) bool {
	return class == collector.ClassWindows
}

func pickCredential(creds []collector.Credential) (collector.Credential, bool) {
	for _, cr := range creds {
		if cr.Kind == "winrm" {
			return cr, true
		}
	}
	return collector.Credential{}, false
}

// Collect runs the inventory script over WinRM and returns a normalized device.
func (c *Collector) Collect(ctx context.Context, t collector.Target) (collector.Result, error) {
	cred, ok := pickCredential(t.Credentials)
	if !ok {
		return collector.Result{}, errors.New("no winrm credential")
	}

	port := c.Port
	if port == 0 {
		if c.UseHTTPS {
			port = 5986
		} else {
			port = 5985
		}
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	endpoint := winrm.NewEndpoint(t.Addr.String(), port, c.UseHTTPS, c.Insecure, nil, nil, nil, timeout)
	client, err := winrm.NewClient(endpoint, cred.Username, cred.Secret)
	if err != nil {
		return collector.Result{}, fmt.Errorf("winrm client: %w", err)
	}

	stdout, stderr, code, err := client.RunWithContextWithString(ctx, winrm.Powershell(inventoryScript), "")
	if err != nil {
		return collector.Result{}, fmt.Errorf("winrm run: %w", err)
	}
	if code != 0 {
		return collector.Result{}, fmt.Errorf("powershell exit %d: %s", code, stderr)
	}

	dev, err := parseInventory([]byte(stdout))
	if err != nil {
		return collector.Result{}, fmt.Errorf("parse inventory: %w", err)
	}
	if !dev.PrimaryIP.IsValid() {
		dev.PrimaryIP = t.Addr
	}

	return collector.Result{Target: t, Device: dev, Raw: map[string]any{"json": stdout}}, nil
}

// inventoryScript gathers Windows inventory and emits one compact JSON object.
// Arrays are forced with @(...) but PowerShell 5.1 may still collapse single
// elements; the parser tolerates both shapes.
const inventoryScript = `
$ErrorActionPreference='SilentlyContinue'
$os=Get-CimInstance Win32_OperatingSystem
$cs=Get-CimInstance Win32_ComputerSystem
$bios=Get-CimInstance Win32_BIOS
$cpu=Get-CimInstance Win32_Processor | Select-Object -First 1
$net=Get-CimInstance Win32_NetworkAdapterConfiguration -Filter 'IPEnabled=True' | ForEach-Object {
  [PSCustomObject]@{ Description=$_.Description; MACAddress=$_.MACAddress; IPAddress=$_.IPAddress } }
$paths='HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*','HKLM:\Software\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*'
$sw=Get-ItemProperty $paths | Where-Object { $_.DisplayName } | ForEach-Object {
  [PSCustomObject]@{ DisplayName=$_.DisplayName; DisplayVersion=$_.DisplayVersion; Publisher=$_.Publisher; InstallDate=$_.InstallDate } }
$users=Get-CimInstance Win32_UserAccount -Filter 'LocalAccount=True' | ForEach-Object {
  [PSCustomObject]@{ Name=$_.Name; FullName=$_.FullName } }
[PSCustomObject]@{
  OS=[PSCustomObject]@{ Caption=$os.Caption; Version=$os.Version; BuildNumber=$os.BuildNumber; OSArchitecture=$os.OSArchitecture }
  Computer=[PSCustomObject]@{ Name=$cs.Name; Domain=$cs.Domain; Manufacturer=$cs.Manufacturer; Model=$cs.Model; TotalPhysicalMemory=$cs.TotalPhysicalMemory }
  BIOS=[PSCustomObject]@{ SerialNumber=$bios.SerialNumber }
  CPU=[PSCustomObject]@{ Name=$cpu.Name; NumberOfLogicalProcessors=$cpu.NumberOfLogicalProcessors }
  Network=@($net)
  Software=@($sw)
  Users=@($users)
} | ConvertTo-Json -Depth 5 -Compress
`
