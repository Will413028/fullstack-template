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
	"sync/atomic"
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

// Readiness reports whether this instance should receive new requests: not once shutdown starts, and not
// while a dependency (the database) is unavailable.
type Readiness struct {
	ping     func(context.Context) error
	draining atomic.Bool
}

// NewReadiness checks dependencies with ping.
func NewReadiness(ping func(context.Context) error) *Readiness {
	return &Readiness{ping: ping}
}

func (r *Readiness) check(ctx context.Context) error {
	if r.draining.Load() {
		return errors.New("shutting down")
	}
	return r.ping(ctx)
}

// Deps are the handler's dependencies, all supplied by the composition root (cmd/app).
type Deps struct {
	Ready  *Readiness
	Logger *slog.Logger
}

// NewHandler builds the public handler.
//
// /health is liveness: 200 while the process runs, without checking dependencies. /health/ready is readiness:
// 503 while a dependency is unavailable or shutdown is in progress.
func NewHandler(deps Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := deps.Ready.check(r.Context()); err != nil {
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

// Serve serves on ln until ctx ends, then shuts down gracefully: readiness turns 503 first and Serve waits
// drain so the proxy takes this instance out of rotation, then closes within shutdownTimeout.
func Serve(ctx context.Context, ln net.Listener, srv *http.Server, ready *Readiness, drain, shutdownTimeout time.Duration, logger *slog.Logger) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	ready.draining.Store(true)
	logger.Info("shutting down", "drain", drain)
	time.Sleep(drain)
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
