package main

import (
	"log/slog"

	"github.com/stock3/motzworks/internal/collector"
	"github.com/stock3/motzworks/internal/collector/fortigate"
	"github.com/stock3/motzworks/internal/collector/fritzbox"
	"github.com/stock3/motzworks/internal/collector/opnsense"
	"github.com/stock3/motzworks/internal/collector/proxmox"
	"github.com/stock3/motzworks/internal/collector/snmp"
	"github.com/stock3/motzworks/internal/collector/ssh"
	"github.com/stock3/motzworks/internal/collector/winrm"
	"github.com/stock3/motzworks/internal/collector/wmi"
)

// registerCollectors wires the available collectors into the registry. Order
// matters for Windows: the engine stops at the first collector that succeeds,
// so WMI (Spiceworks-style WMI/DCOM, works on hardened/Kerberos-only hosts)
// comes before WinRM.
func registerCollectors(reg *collector.Registry, log *slog.Logger) {
	reg.Register(ssh.New(log))
	reg.Register(snmp.New(log))
	reg.Register(wmi.New(log))
	reg.Register(winrm.New(log))
	reg.Register(proxmox.New(log))
	reg.Register(opnsense.New(log))
	reg.Register(fortigate.New(log))
	reg.Register(fritzbox.New(log))
}
