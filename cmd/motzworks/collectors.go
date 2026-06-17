package main

import (
	"log/slog"

	"github.com/stock3/motzworks/internal/collector"
	"github.com/stock3/motzworks/internal/collector/snmp"
	"github.com/stock3/motzworks/internal/collector/ssh"
	"github.com/stock3/motzworks/internal/collector/winrm"
)

// registerCollectors wires the available collectors into the registry.
func registerCollectors(reg *collector.Registry, log *slog.Logger) {
	reg.Register(ssh.New(log))
	reg.Register(snmp.New(log))
	reg.Register(winrm.New(log))
}
