// Command app is the backend's single binary. Subcommands: serve (HTTP API), migrate (apply migrations as the owner
// role and grant the app role) and healthcheck (the container health probe; the distroless image has no shell or
// curl).
// This is the composition root: settings are read and validated once here, and platform and modules are wired.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Will413028/fullstack-template/backend/internal/httpapi"
	"github.com/Will413028/fullstack-template/backend/internal/platform/config"
	"github.com/Will413028/fullstack-template/backend/internal/platform/db"
	"github.com/Will413028/fullstack-template/backend/internal/platform/logx"
	"github.com/Will413028/fullstack-template/backend/internal/platform/migrate"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = "usage: app serve|migrate|healthcheck"

// Where the image puts the Atlas community build and the migrations (Dockerfile).
const (
	atlasBin      = "/usr/local/bin/atlas"
	migrationsDir = "/app/migrations"
)

// shutdownTimeout lets the longest legitimate request (httpapi.DefaultTimeouts.Write) finish; compose's
// stop_grace_period is a few seconds longer.
const shutdownTimeout = 30 * time.Second

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1])
	stop()
	if err != nil {
		slog.Error("exit", "command", os.Args[1], "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, command string) error {
	switch command {
	case "serve":
		return serve(ctx)
	case "migrate":
		return runMigrate(ctx)
	case "healthcheck":
		addr := os.Getenv("HTTP_ADDR")
		if addr == "" {
			addr = ":8000"
		}
		return healthcheck(ctx, probeAddr(addr))
	}
	return errors.New(usage)
}

func serve(ctx context.Context) error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	logger := logx.New(os.Stdout, cfg.Mode)
	pool, err := db.Open(ctx, db.Config{
		URL:              cfg.DatabaseURL,
		Password:         cfg.DatabasePassword,
		MaxConns:         cfg.DBMaxConns,
		StatementTimeout: cfg.DBStatementTimeout,
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	h, err := httpapi.NewHandler(ctx, httpapi.Deps{Ready: pool.Ping, Logger: logger, Docs: cfg.Mode == "dev"})
	if err != nil {
		return err
	}
	srv := httpapi.NewServer(h, httpapi.DefaultTimeouts)
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		return err
	}
	logger.Info("serving", "app", cfg.AppName, "version", version, "addr", ln.Addr().String(), "mode", cfg.Mode)
	return httpapi.Serve(ctx, ln, srv, shutdownTimeout, logger)
}

func runMigrate(ctx context.Context) error {
	cfg, err := config.LoadMigrate(os.LookupEnv)
	if err != nil {
		return err
	}
	logger := logx.New(os.Stdout, cfg.Mode)
	err = migrate.Run(ctx, migrate.Config{
		URL:           cfg.DatabaseURL,
		Password:      cfg.DatabasePassword,
		AppRole:       cfg.AppRole,
		AtlasBin:      atlasBin,
		MigrationsDir: migrationsDir,
	})
	if err != nil {
		return err
	}
	logger.Info("migrated", "version", version, "app_role", cfg.AppRole)
	return nil
}

// healthcheck asks the local server whether it is ready (GET /health/ready) and fails unless it answers 200.
func healthcheck(ctx context.Context, addr string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	// addr comes from HTTP_ADDR, deployment configuration rather than user input.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/health/ready", nil) //nolint:gosec // see above
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // addr is deployment configuration
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("not ready: %s", resp.Status)
	}
	return nil
}

// probeAddr turns a listen address into one to dial: an empty or unspecified host means this machine.
func probeAddr(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return listen
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
