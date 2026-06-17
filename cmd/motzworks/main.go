// Command motzworks is the agentless network inventory scanner.
//
// Phase 0 provides the foundation commands: version, config check, database
// migration, and vault key generation. Discovery, collectors, the scheduler,
// API and dashboards arrive in later phases.
package main

import (
	"fmt"
	"os"

	"github.com/stock3/motzworks/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	args := os.Args[2:]
	var err error
	switch os.Args[1] {
	case "version", "-v", "--version":
		fmt.Printf("motzworks %s\n", version.Version)
	case "config":
		err = cmdConfig(args)
	case "migrate":
		err = cmdMigrate(args)
	case "scan":
		err = cmdScan(args)
	case "serve":
		err = cmdServe(args)
	case "user":
		err = cmdUser(args)
	case "vault":
		err = cmdVault(args)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `motzworks — agentless network inventory scanner

Usage:
  motzworks <command> [flags]

Commands:
  version              Print the version
  config check         Load and validate a config file
  migrate up           Apply pending database migrations
  scan                 Discover and inventory hosts (-targets <cidr,...>)
  serve                Run the API server, scheduler and web dashboard
  user add             Create or update a dashboard user
  vault genkey         Generate a new base64 vault key
  help                 Show this help

Common flags:
  -config <path>       Config file (default "config.yaml")
`)
}
