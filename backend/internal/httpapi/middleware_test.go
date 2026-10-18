package httpapi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Will413028/fullstack-template/backend/internal/platform/logx"
)

var proxies = []netip.Prefix{netip.MustParsePrefix("172.30.0.0/24")}

const (
	proxyAddr     = "172.30.0.2:41000"
	untrustedAddr = "203.0.113.7:41000"
)

// doFrom sends a request from the peer remote with extra headers.
func doFrom(t *testing.T, h http.Handler, method, target, remote string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body := http.NoBody
	req := httptest.NewRequestWithContext(t.Context(), method, "http://app.test"+target, body)
	if method == http.MethodPost {
		req = httptest.NewRequestWithContext(t.Context(), method, "http://app.test"+target, strings.NewReader(`{"name":"abc"}`))
		req.Header.Set("Content-Type", "application/json")
	}
	req.RemoteAddr = remote
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func assertGeneratedRequestID(t *testing.T, rec *httptest.ResponseRecorder, sent string) {
	t.Helper()
	got := rec.Header().Get(headerRequestID)
	if got == sent {
		t.Fatalf("X-Request-ID = %q: the sent value was used", got)
	}
	if id, err := uuid.Parse(got); err != nil || id.Version() != 7 {
		t.Fatalf("X-Request-ID = %q, want a UUIDv7", got)
	}
}

func TestRequestIDFromTrustedProxy(t *testing.T) {
	h := newStubAPI(t, Deps{TrustedProxies: proxies})
	rec := doFrom(t, h, http.MethodGet, "/api/widgets", proxyAddr, map[string]string{headerRequestID: "abc-123"})
	if got := rec.Header().Get(headerRequestID); got != "abc-123" {
		t.Fatalf("X-Request-ID = %q, want the proxy's abc-123", got)
	}
}

func TestRequestIDFromUntrustedPeerReplaced(t *testing.T) {
	h := newStubAPI(t, Deps{TrustedProxies: proxies})
	rec := doFrom(t, h, http.MethodGet, "/api/widgets", untrustedAddr, map[string]string{headerRequestID: "abc-123"})
	assertGeneratedRequestID(t, rec, "abc-123")
}

func TestRequestIDMalformedReplaced(t *testing.T) {
	h := newStubAPI(t, Deps{TrustedProxies: proxies})
	for name, id := range map[string]string{
		"too long":      strings.Repeat("a", 65),
		"space":         "abc 123",
		"log injection": `x" level=ERROR msg="forged`,
		"underscore":    "abc_123",
	} {
		t.Run(name, func(t *testing.T) {
			rec := doFrom(t, h, http.MethodGet, "/api/widgets", proxyAddr, map[string]string{headerRequestID: id})
			assertGeneratedRequestID(t, rec, id)
		})
	}
	rec := doFrom(t, h, http.MethodGet, "/api/widgets", proxyAddr, map[string]string{headerRequestID: strings.Repeat("a", 64)})
	if got := rec.Header().Get(headerRequestID); got != strings.Repeat("a", 64) {
		t.Fatalf("64-character id replaced by %q", got)
	}
}

func TestProblemCarriesRequestID(t *testing.T) {
	h := newStubAPI(t, Deps{})
	rec := do(t, h, http.MethodGet, "/api/nope", "", "")
	var body struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.RequestID == "" || body.RequestID != rec.Header().Get(headerRequestID) {
		t.Fatalf("request_id = %q, X-Request-ID = %q", body.RequestID, rec.Header().Get(headerRequestID))
	}
}

// logLines decodes JSON log output into one map per line.
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	sc := bufio.NewScanner(buf)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var line map[string]any
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			t.Fatalf("log line %q: %v", sc.Text(), err)
		}
		lines = append(lines, line)
	}
	return lines
}

func findLog(t *testing.T, lines []map[string]any, msg string) map[string]any {
	t.Helper()
	for _, l := range lines {
		if l["msg"] == msg {
			return l
		}
	}
	t.Fatalf("no %q log line in %v", msg, lines)
	return nil
}

// The panic log is written inside recoverPanics; it carries the request id only because withRequestInfo is outside.
func TestPanicLogAndAccessLogCarryRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(logx.WithContext(slog.NewJSONHandler(&buf, nil)))
	h := newStubAPI(t, Deps{Logger: logger, TrustedProxies: proxies})
	rec := doFrom(t, h, http.MethodGet, "/api/fail?cursor=secret", proxyAddr,
		map[string]string{"X-Forwarded-For": "198.51.100.9"})
	id := rec.Header().Get(headerRequestID)
	lines := logLines(t, &buf)

	if got := findLog(t, lines, "panic")["request_id"]; got != id {
		t.Errorf("panic log request_id = %v, want %q", got, id)
	}
	access := findLog(t, lines, "request")
	want := map[string]any{
		"request_id": id, "method": "GET", "path": "/api/fail", "status": float64(500), "client_ip": "198.51.100.9",
	}
	for k, v := range want {
		if access[k] != v {
			t.Errorf("access log %s = %v, want %v", k, access[k], v)
		}
	}
	if _, ok := access["duration_ms"].(float64); !ok {
		t.Errorf("access log duration_ms = %v, want a number", access["duration_ms"])
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := newStubAPI(t, Deps{})
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"ok":           do(t, h, http.MethodGet, "/api/widgets", "", ""),
		"problem":      do(t, h, http.MethodGet, "/api/nope", "", ""),
		"cross-origin": doFrom(t, h, http.MethodPost, "/api/widgets", untrustedAddr, map[string]string{"Sec-Fetch-Site": "cross-site"}),
	} {
		t.Run(name, func(t *testing.T) {
			for k, v := range map[string]string{
				"X-Content-Type-Options": "nosniff", "Cache-Control": "no-store", "Referrer-Policy": "no-referrer",
			} {
				if got := rec.Header().Get(k); got != v {
					t.Errorf("%s = %q, want %q", k, got, v)
				}
			}
			for _, k := range []string{"X-XSS-Protection", "X-Frame-Options", "Permissions-Policy"} {
				if got := rec.Header().Get(k); got != "" {
					t.Errorf("%s = %q, want absent", k, got)
				}
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	tr := trust(proxies)
	for _, c := range []struct {
		name, remote string
		xff          []string
		want         string
	}{
		{"untrusted peer ignores XFF", untrustedAddr, []string{"198.51.100.9"}, "203.0.113.7"},
		{"trusted peer without XFF", proxyAddr, nil, "172.30.0.2"},
		{"rightmost untrusted hop", proxyAddr, []string{"192.0.2.1, 198.51.100.9"}, "198.51.100.9"},
		{"skips trusted hops", proxyAddr, []string{"198.51.100.9, 172.30.0.3"}, "198.51.100.9"},
		{"several XFF headers", proxyAddr, []string{"192.0.2.1", "198.51.100.9"}, "198.51.100.9"},
		{"garbage stops at the last real hop", proxyAddr, []string{"198.51.100.9, garbage, 172.30.0.3"}, "172.30.0.3"},
		{"IPv4-mapped IPv6", "[::ffff:172.30.0.2]:41000", []string{"198.51.100.9"}, "198.51.100.9"},
	} {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
			req.RemoteAddr = c.remote
			for _, v := range c.xff {
				req.Header.Add("X-Forwarded-For", v)
			}
			if got := tr.clientIP(req).String(); got != c.want {
				t.Fatalf("clientIP = %s, want %s", got, c.want)
			}
		})
	}
}

func TestAccessLogAbortBeforeWriting(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(logx.WithContext(slog.NewJSONHandler(&buf, nil)))
	h := protect(Deps{Logger: logger}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	func() {
		defer func() { _ = recover() }()
		do(t, h, http.MethodGet, "/x", "", "")
	}()
	if got := findLog(t, logLines(t, &buf), "request aborted")["status"]; got != float64(0) {
		t.Fatalf("aborted request status = %v, want 0 (nothing was sent)", got)
	}
}

// Handlers reach the connection's Flush and deadlines through http.ResponseController, which needs every wrapper in
// the chain to Unwrap.
func TestResponseControllerReachesConnection(t *testing.T) {
	var flushErr error
	h := protect(Deps{Logger: discard}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flushErr = http.NewResponseController(w).Flush()
	}))
	rec := do(t, h, http.MethodGet, "/x", "", "")
	if flushErr != nil || !rec.Flushed {
		t.Fatalf("Flush through the middleware chain: err = %v, flushed = %v", flushErr, rec.Flushed)
	}
}
