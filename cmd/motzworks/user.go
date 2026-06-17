package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/stock3/motzworks/internal/auth"
	"github.com/stock3/motzworks/internal/config"
	"github.com/stock3/motzworks/internal/store"
)

// cmdUser implements: motzworks user add -username u -password p [-role admin]
func cmdUser(args []string) error {
	if len(args) == 0 || args[0] != "add" {
		return fmt.Errorf("usage: motzworks user add -username <u> -password <p> [-role admin|viewer]")
	}

	fs := flag.NewFlagSet("user add", flag.ExitOnError)
	cfgPath := fs.String("config", "config.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	role := fs.String("role", "viewer", "role: admin or viewer")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *username == "" || *password == "" {
		return fmt.Errorf("user add requires -username and -password")
	}
	normRole, err := auth.NormalizeRole(*role)
	if err != nil {
		return err
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(*password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	st, err := store.Open(ctx, cfg.Database.DSN())
	if err != nil {
		return err
	}
	defer st.Close()

	id, err := st.CreateUser(ctx, *username, hash, normRole)
	if err != nil {
		return err
	}
	fmt.Printf("user %q (%s) saved: %s\n", *username, normRole, id)
	return nil
}
