package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/stock3/motzworks/internal/config"
	"github.com/stock3/motzworks/internal/logging"
	"github.com/stock3/motzworks/internal/store"
	"github.com/stock3/motzworks/internal/vault"
)

// cmdConfig implements: motzworks config check [-config path]
func cmdConfig(args []string) error {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	path := fs.String("config", "config.yaml", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if sub := fs.Arg(0); sub != "" && sub != "check" {
		return fmt.Errorf("usage: motzworks config check [-config path]")
	}

	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	fmt.Printf("config OK (%s)\n", *path)
	fmt.Printf("  log:      level=%s format=%s\n", cfg.Log.Level, cfg.Log.Format)
	fmt.Printf("  database: %s:%d/%s (sslmode=%s)\n",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.Name, cfg.Database.SSLMode)
	fmt.Printf("  server:   addr=%s\n", cfg.Server.Addr)
	fmt.Printf("  scan:     concurrency=%d\n", cfg.Scan.Concurrency)
	return nil
}

// cmdMigrate implements: motzworks migrate up [-config path]
func cmdMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	path := fs.String("config", "config.yaml", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if sub := fs.Arg(0); sub != "" && sub != "up" {
		return fmt.Errorf("usage: motzworks migrate up [-config path]")
	}

	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	log := logging.New(cfg.Log.Level, cfg.Log.Format)

	ctx := context.Background()
	st, err := store.Open(ctx, cfg.Database.DSN())
	if err != nil {
		return err
	}
	defer st.Close()

	applied, err := st.Migrate(ctx)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		log.Info("no pending migrations")
	} else {
		log.Info("applied migrations", "count", len(applied), "versions", applied)
	}
	return nil
}

// cmdVault implements: motzworks vault genkey
func cmdVault(args []string) error {
	fs := flag.NewFlagSet("vault", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch fs.Arg(0) {
	case "genkey":
		key, err := vault.GenerateKey()
		if err != nil {
			return err
		}
		fmt.Println(key)
		return nil
	default:
		return fmt.Errorf("usage: motzworks vault genkey")
	}
}
