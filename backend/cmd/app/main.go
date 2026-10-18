// Command app is the backend's single binary. Subcommands: serve (HTTP API).
// This is the composition root: settings are read and validated once here, and platform and modules are wired.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Will413028/fullstack-template/backend/internal/httpapi"
	"github.com/Will413028/fullstack-template/backend/internal/platform/config"
	"github.com/Will413028/fullstack-template/backend/internal/platform/db"
	"github.com/Will413028/fullstack-template/backend/internal/platform/logx"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = "usage: app serve"

// Shutdown timing: readiness turns 503 and waits drainDelay, then the server closes within shutdownTimeout.
const (
	drainDelay      = 5 * time.Second
	shutdownTimeout = 20 * time.Second
)

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
		MaxConns:         cfg.DBMaxConns,
		StatementTimeout: cfg.DBStatementTimeout,
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	ready := httpapi.NewReadiness(pool.Ping)
	srv := httpapi.NewServer(httpapi.NewHandler(httpapi.Deps{Ready: ready, Logger: logger}), httpapi.DefaultTimeouts)
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		return err
	}
	logger.Info("serving", "app", cfg.AppName, "version", version, "addr", ln.Addr().String(), "mode", cfg.Mode)
	return httpapi.Serve(ctx, ln, srv, ready, drainDelay, shutdownTimeout, logger)
}
