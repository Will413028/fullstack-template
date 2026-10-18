// Package httpapi is the HTTP entry point: routing, cross-cutting middleware, http.Server settings and graceful
// shutdown. It handles transport only; business rules live in internal/modules.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// maxBodyBytes caps every request body.
const maxBodyBytes = 1 << 20

// Timeouts are the http.Server timeouts.
type Timeouts struct {
	ReadHeader time.Duration
	Read       time.Duration
	Write      time.Duration
	Idle       time.Duration
}

// DefaultTimeouts are the production timeouts.
var DefaultTimeouts = Timeouts{
	ReadHeader: 5 * time.Second,
	Read:       15 * time.Second,
	Write:      30 * time.Second,
	Idle:       120 * time.Second,
}

// NewServer returns an http.Server with the given timeouts; the caller's listener decides the address.
func NewServer(h http.Handler, t Timeouts) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: t.ReadHeader,
		ReadTimeout:       t.Read,
		WriteTimeout:      t.Write,
		IdleTimeout:       t.Idle,
	}
}

// Deps are the handler's dependencies, all supplied by the composition root (cmd/app).
type Deps struct {
	// Ready reports whether dependencies (the database) are available.
	Ready  func(context.Context) error
	Logger *slog.Logger
}

// NewHandler builds the public handler.
//
// /health is liveness: 200 while the process runs, without checking dependencies. /health/ready is readiness:
// 503 while a dependency is unavailable.
func NewHandler(deps Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := deps.Ready(r.Context()); err != nil {
			deps.Logger.WarnContext(r.Context(), "not ready", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	return protect(mux)
}

// protect applies the protections every request gets:
//   - CrossOriginProtection rejects cross-origin state-changing requests (CSRF), using Sec-Fetch-Site and
//     falling back to comparing Origin with Host. Non-browser clients that send neither header pass.
//     GET, HEAD and OPTIONS are not checked.
//   - The body limit: reading past maxBodyBytes returns *http.MaxBytesError.
func protect(h http.Handler) http.Handler {
	limited := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		h.ServeHTTP(w, r)
	})
	return http.NewCrossOriginProtection().Handler(limited)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// Serve serves on ln until ctx ends, then shuts down gracefully: Shutdown stops accepting connections and waits up
// to shutdownTimeout for in-flight requests.
//
// There is no readiness drain before Shutdown: with one backend behind Caddy no load balancer reads readiness to
// move traffic, and Caddy's lb_try_duration holds requests while the backend restarts. Add a drain together with
// zero-downtime deploys, when the backend runs more than one replica (docs/adr/2026-10-18-api-and-auth-baseline.md).
func Serve(ctx context.Context, ln net.Listener, srv *http.Server, shutdownTimeout time.Duration, logger *slog.Logger) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	logger.Info("shutting down", "timeout", shutdownTimeout)
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
