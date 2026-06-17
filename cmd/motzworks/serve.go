package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stock3/motzworks/internal/api"
	"github.com/stock3/motzworks/internal/auth"
	"github.com/stock3/motzworks/internal/collector"
	"github.com/stock3/motzworks/internal/config"
	"github.com/stock3/motzworks/internal/logging"
	"github.com/stock3/motzworks/internal/scan"
	"github.com/stock3/motzworks/internal/scheduler"
	"github.com/stock3/motzworks/internal/store"
	"github.com/stock3/motzworks/internal/vault"
	"github.com/stock3/motzworks/internal/web"
)

// cmdServe runs the HTTP API, the embedded dashboard and the scan scheduler.
func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cfgPath := fs.String("config", "config.yaml", "config file")
	addrOverride := fs.String("addr", "", "override server.addr (e.g. :8080)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	log := logging.New(cfg.Log.Level, cfg.Log.Format)

	// The vault key must be stable so stored credentials remain decryptable.
	v, err := vault.FromEnv(cfg.Vault.KeyEnv)
	if err != nil {
		return fmt.Errorf("vault: %w (generate one with `motzworks vault genkey` and export %s)", err, cfg.Vault.KeyEnv)
	}

	secret := cfg.Auth.Secret
	if secret == "" {
		secret, _ = auth.GenerateSecret()
		log.Warn("no auth secret configured; using an ephemeral one — sessions reset on restart (set MOTZWORKS_AUTH_SECRET)")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	st, err := store.Open(ctx, cfg.Database.DSN())
	if err != nil {
		return err
	}
	defer st.Close()

	if n, err := st.CountUsers(ctx); err == nil && n == 0 {
		log.Warn("no dashboard users exist; create one with: motzworks user add -username admin -password <pw> -role admin")
	}

	reg := collector.NewRegistry()
	registerCollectors(reg, log)
	engine := scan.New(st, reg, log)

	sched := scheduler.New(st, engine, v, log, 30*time.Second, cfg.Scan.Concurrency,
		cfg.Scan.RatePerSec, time.Duration(cfg.Scan.JitterMs)*time.Millisecond)
	go sched.Run(ctx)

	static, err := web.Handler()
	if err != nil {
		return fmt.Errorf("web assets: %w", err)
	}

	apiSrv := api.New(st, engine, v, log, api.Options{
		Secret:      []byte(secret),
		SessionTTL:  time.Duration(cfg.Auth.SessionHours) * time.Hour,
		Concurrency: cfg.Scan.Concurrency,
		Static:      static,
	})

	addr := cfg.Server.Addr
	if *addrOverride != "" {
		addr = *addrOverride
	}
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           apiSrv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	log.Info("motzworks serving", "addr", addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	log.Info("shutdown complete")
	return nil
}
