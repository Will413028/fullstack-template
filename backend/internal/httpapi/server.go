// Package httpapi is the HTTP entry point: routing, cross-cutting middleware, http.Server settings and graceful
// shutdown. It handles transport only; business rules live in internal/modules.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Will413028/template-go-nextjs/backend/api"
	"github.com/Will413028/template-go-nextjs/backend/internal/gen/app/sessions/v1/sessionsv1connect"
	"github.com/Will413028/template-go-nextjs/backend/internal/gen/app/users/v1/usersv1connect"
	"github.com/Will413028/template-go-nextjs/backend/internal/httpapi/gen"
	"github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc"
	"github.com/Will413028/template-go-nextjs/backend/internal/modules/auth"
	"github.com/Will413028/template-go-nextjs/backend/internal/modules/items"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/apperr"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/authctx"
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
	// Docs serves the contract at /api/openapi.yaml and Swagger UI at /api/docs (MODE=dev only).
	Docs bool
	// TrustedProxies are the peers whose X-Forwarded-For and X-Request-ID are believed (TRUSTED_PROXIES).
	TrustedProxies []netip.Prefix
	// Now is the clock for rate limiting and request durations; the composition root passes time.Now.
	Now func() time.Time
	// RateLimitPerMin is the per-IP limit on the procedures connectLimited names (AUTH_RATE_LIMIT_PER_MIN).
	RateLimitPerMin int
	// Auth serves the user and session services. Nil only in tests that call none of their methods and send no session
	// cookie.
	Auth   *auth.Service
	Cookie Cookie
	// Items serves ItemAppService. Nil only in tests that call none of its methods.
	Items *items.Service

	// testServices are Connect services mounted next to connectServices' (tests only).
	testServices []rpc.Service
	// testLimited are procedures added to connectLimited (tests only).
	testLimited []string
}

// apiServer implements the operations in api/openapi.yaml (gen.StrictServerInterface), which has none left: every
// module is served by Connect. docs/plans/2026-11-03-connect-contract.md step 5 deletes the REST layer.
type apiServer struct{}

// connectServices are the Connect services NewHandler mounts.
func connectServices(deps Deps) []rpc.Service {
	services := []rpc.Service{itemsService(deps.Items)}
	services = append(services, userServices(deps.Auth, deps.Cookie)...)
	services = append(services, sessionServices(deps.Auth, deps.Cookie)...)
	return append(services, deps.testServices...)
}

// connectLimited are the procedures the per-client rate limit applies to at the Connect entry (connectEntry): sign-up
// and sign-in (docs/adr/2026-10-18-api-and-auth-baseline.md D6); sign-in is also throttled per account in the auth
// module.
var connectLimited = []string{
	usersv1connect.UserPublicServiceCreateUserProcedure,
	sessionsv1connect.SessionPublicServiceCreateSessionProcedure,
}

// NewHandler builds the public handler: the API under /api (Connect from connectServices; the REST contract has no
// operation left), and the health checks.
func NewHandler(ctx context.Context, deps Deps) (http.Handler, error) {
	if deps.RateLimitPerMin < 1 {
		return nil, errors.New("RateLimitPerMin must be positive")
	}
	if deps.Now == nil {
		return nil, errors.New("Now is required")
	}
	spec, err := loadSpec(ctx, api.Spec)
	if err != nil {
		return nil, fmt.Errorf("load api/openapi.yaml: %w", err)
	}
	routes := http.NewServeMux()
	strict := gen.NewStrictHandlerWithOptions(apiServer{}, nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeBodyDecodeError(w, r, deps.Logger, err)
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, r, deps.Logger, err)
		},
	})
	gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseURL:    "/api",
		BaseRouter: routes,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeParamError(w, r, deps.Logger, err)
		},
	})
	connectMux, err := rpc.NewMux(deps.Logger, auth.RoleAdmin, connectServices(deps))
	if err != nil {
		return nil, err
	}
	connectLimit := map[string]bool{}
	for _, p := range append(append([]string(nil), connectLimited...), deps.testLimited...) {
		if _, ok := connectMux.Class(p); !ok {
			return nil, fmt.Errorf("rate limit: %s is not a mounted procedure", p)
		}
		connectLimit[p] = true
	}
	limiter := newIPLimiter(deps.RateLimitPerMin, deps.Now)
	connect := connectEntry(deps.Logger, connectMux, limiter, connectLimit)
	return assemble(deps, spec, routes, connect), nil
}

// connectPrefix starts every Connect path: /api/<package>.<Service>/<Method>, and every package is under the proto
// root app (decision D5). The transition mux (assemble) and writeError choose Connect by this one condition.
const connectPrefix = "/api/app."

func isConnect(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, connectPrefix) }

var errNoProcedure = apperr.New(apperr.NotFound, "", "no such procedure")

// connectEntry is what the net/http layer checks for a Connect request before connect-go decodes it; connect-go runs
// interceptors only after decoding, so a check that must come first cannot be an interceptor. In order:
//   - a procedure that is not mounted is not_found, without asking for a session;
//   - App and Admin procedures need a session (authenticate resolved it): unauthenticated otherwise, so an anonymous
//     caller learns nothing from decoding or field errors;
//   - the procedures in limited take a token from the client's bucket: resource_exhausted with RetryInfo when it is
//     empty, so a malformed request uses a token too.
//
// The admin role is checked by an interceptor (rpc.NewMux): only a signed-in user reaches it.
func connectEntry(logger *slog.Logger, m *rpc.Mux, limiter *ipLimiter, limited map[string]bool) http.Handler {
	handler := http.StripPrefix("/api", m)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		procedure := strings.TrimPrefix(r.URL.Path, "/api")
		class, ok := m.Class(procedure)
		if !ok {
			writeError(w, r, logger, errNoProcedure)
			return
		}
		if _, signedIn := authctx.From(r.Context()); class != rpc.Public && !signedIn {
			writeError(w, r, logger, auth.ErrSessionInvalid)
			return
		}
		if limited[procedure] {
			if d := limiter.wait(clientIPFrom(r.Context())); d > 0 {
				writeError(w, r, logger, &apperr.Error{Kind: apperr.RateLimited, Detail: "too many requests from this address",
					RetryAfter: d})
				return
			}
		}
		handler.ServeHTTP(w, r)
	})
}

// assemble mounts the API behind authentication, then splits it: Connect paths (isConnect) go to connect, every
// other /api path to the REST routes behind request validation against spec. It adds the health checks and wraps
// everything in the protections every request gets.
//
// /health is liveness: 200 while the process runs, without checking dependencies. /health/ready is readiness:
// 503 while a dependency is unavailable. Both sit outside the contract and outside validation.
func assemble(deps Deps, spec *openapi3.T, apiRoutes, connect http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := deps.Ready(r.Context()); err != nil {
			deps.Logger.WarnContext(r.Context(), "not ready", "error", err)
			writeProblem(w, problemUnavailable, "", "")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	rest := validateRequests(spec, deps.Logger, apiRoutes)
	apiHandler := authenticate(deps.Auth, deps.Cookie, deps.Logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isConnect(r) {
			connect.ServeHTTP(w, r)
			return
		}
		rest.ServeHTTP(w, r)
	}))
	if deps.Docs {
		apiHandler = withDocs(apiHandler)
	}
	mux.Handle("/api/", apiHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeProblem(w, problemNotFound, "", "")
	})
	return protect(deps, mux)
}

// protect applies the protections every request gets, outermost first:
//   - withRequestInfo resolves the client address and the request id. It is outermost so that every log line,
//     the panic log included, and every response, problems included, carry the request id.
//   - accessLog logs the request once it ends, with the status recoverPanics chose.
//   - securityHeaders, set before any handler writes.
//   - A panic becomes a logged internal error.
//   - CrossOriginProtection rejects cross-origin state-changing requests (CSRF) with permission_denied
//     (CROSS_ORIGIN_REQUEST; a 403 problem for REST), using Sec-Fetch-Site and falling back to comparing Origin with
//     Host. Non-browser clients that send neither header pass. GET, HEAD and OPTIONS are not checked, which is why
//     only Get and List methods may be served on GET (rpc.NewMux).
//   - The body limit: reading past maxBodyBytes returns *http.MaxBytesError, which writeError turns into 413 and
//     connect-go into resource_exhausted (without RetryInfo, which tells it from the rate limit).
//
// Errors are written by writeError, in the format of the request: Connect or REST.
//
// A middleware that wraps the ResponseWriter must provide Unwrap: responseStarted walks the chain to find
// recoverPanics' startWriter.
func protect(deps Deps, h http.Handler) http.Handler {
	limited := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		h.ServeHTTP(w, r)
	})
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, deps.Logger, errCrossOrigin)
	}))
	inner := recoverPanics(deps.Logger, cop.Handler(limited))
	return withRequestInfo(deps.TrustedProxies, accessLog(deps.Logger, deps.Now, securityHeaders(inner)))
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
