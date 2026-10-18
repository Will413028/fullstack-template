package httpapi

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Will413028/fullstack-template/backend/internal/platform/logx"
)

const headerRequestID = "X-Request-ID"

// validRequestID bounds a request id a trusted proxy may send: it ends up in logs and response headers.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

type clientIPKey struct{}

// clientIPFrom returns the client address that withRequestInfo resolved.
func clientIPFrom(ctx context.Context) netip.Addr {
	addr, _ := ctx.Value(clientIPKey{}).(netip.Addr)
	return addr
}

// trust decides which peers are proxies whose X-Forwarded-For and X-Request-ID are believed (TRUSTED_PROXIES).
type trust []netip.Prefix

func (t trust) contains(addr netip.Addr) bool {
	for _, p := range t {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// peerAddr is the address of the connection's other end.
func peerAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, _ := netip.ParseAddr(host)
	return addr.Unmap()
}

// clientIP is the peer, unless the peer is a trusted proxy: then it is the first X-Forwarded-For address from the
// right that is not a trusted proxy. Entries left of it are whatever the client wrote and are ignored. An entry
// that does not parse stops the walk at the last address known to be real.
func (t trust) clientIP(r *http.Request) netip.Addr {
	addr := peerAddr(r)
	if !t.contains(addr) {
		return addr
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return addr
		}
		addr = hop.Unmap()
		if !t.contains(addr) {
			return addr
		}
	}
	return addr
}

// withRequestInfo resolves the client address and the request id before anything else runs. The request id is
// the one a trusted proxy sent, when well formed, or a new UUIDv7; it goes into the context (so every log line of
// the request carries it), the X-Request-ID response header and every problem body.
func withRequestInfo(t trust, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerRequestID)
		if !t.contains(peerAddr(r)) || !validRequestID.MatchString(id) {
			id = uuid.Must(uuid.NewV7()).String()
		}
		w.Header().Set(headerRequestID, id)
		ctx := logx.WithRequestID(r.Context(), id)
		ctx = context.WithValue(ctx, clientIPKey{}, t.clientIP(r))
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

// securityHeaders sets the headers OWASP's HTTP Headers Cheat Sheet recommends for a JSON API. X-Frame-Options,
// Permissions-Policy and X-XSS-Protection do nothing for JSON (the last one enables an XS-Leak); HSTS is Caddy's.
func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := w.Header()
		hdr.Set("X-Content-Type-Options", "nosniff")
		hdr.Set("Cache-Control", "no-store")
		hdr.Set("Referrer-Policy", "no-referrer")
		h.ServeHTTP(w, r)
	})
}

// accessLog logs one line per request once it ends. The query string is left out: it can hold cursors and search
// terms. Health checks log at debug level: the container probe calls them every few seconds.
func accessLog(logger *slog.Logger, h http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w := &statusWriter{ResponseWriter: rw}
		level := slog.LevelInfo
		if r.URL.Path == "/health" || strings.HasPrefix(r.URL.Path, "/health/") {
			level = slog.LevelDebug
		}
		defer func() {
			// Only http.ErrAbortHandler gets past recoverPanics: the response was cut off, and status is what was
			// sent before that (0 for nothing). A handler that returns without writing gets net/http's implicit 200.
			aborted := recover()
			status := w.code
			if aborted == nil && status == 0 {
				status = http.StatusOK
			}
			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
				slog.String("client_ip", clientIPFrom(r.Context()).String()),
			}
			if aborted != nil {
				logger.LogAttrs(r.Context(), slog.LevelWarn, "request aborted", attrs...)
				panic(aborted)
			}
			logger.LogAttrs(r.Context(), level, "request", attrs...)
		}()
		h.ServeHTTP(w, r)
	})
}

// statusWriter records the status code sent, 0 until something is written.
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.code == 0 && code >= 200 {
		w.code = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.code == 0 {
		w.code = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController and responseStarted reach the underlying writer.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
