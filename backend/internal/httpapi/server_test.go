package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var discard = slog.New(slog.DiscardHandler)

func newTestHandler(t *testing.T, ping func(context.Context) error) http.Handler {
	t.Helper()
	return newTestHandlerDeps(t, Deps{Ready: ping, Logger: discard})
}

func ok(context.Context) error { return nil }

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return rec
}

func assertStatusBody(t *testing.T, rec *httptest.ResponseRecorder, code int, status string) {
	t.Helper()
	if rec.Code != code {
		t.Fatalf("code = %d, want %d (body %q)", rec.Code, code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var body struct{ Status string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Status != status {
		t.Errorf("body = %q, want status %q", rec.Body, status)
	}
}

func TestHealth(t *testing.T) {
	h := newTestHandler(t, func(context.Context) error { return errors.New("db down") })
	// Liveness ignores dependencies: still 200 with the database down.
	assertStatusBody(t, get(t, h, "/health"), http.StatusOK, "ok")
}

func TestReady(t *testing.T) {
	h := newTestHandler(t, ok)
	assertStatusBody(t, get(t, h, "/health/ready"), http.StatusOK, "ready")
}

func TestReadyDBDown(t *testing.T) {
	h := newTestHandler(t, func(context.Context) error { return errors.New("db down") })
	assertProblem(t, get(t, h, "/health/ready"), http.StatusServiceUnavailable, "/problems/unavailable")
}

func TestShutdownFinishesInFlightRequest(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	})
	srv := NewServer(mux, DefaultTimeouts)
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, srv, 5*time.Second, discard) }()

	type result struct {
		code int
		err  error
	}
	got := make(chan result, 1)
	go func() {
		code, err := fetchStatus(t.Context(), "http://"+ln.Addr().String()+"/slow")
		got <- result{code, err}
	}()
	<-started
	cancel() // what signal.NotifyContext does on SIGTERM
	select {
	case err := <-done:
		t.Fatalf("Serve returned before the in-flight request finished: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	if r := <-got; r.err != nil || r.code != http.StatusOK {
		t.Fatalf("in-flight request: code = %d, err = %v, want 200", r.code, r.err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after shutdown")
	}
}

// noKeepAlive dials a fresh connection per request. The default client's pool can leave a spare connection that
// never sends a request; Shutdown treats such a StateNew connection as active for 5s.
var noKeepAlive = &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

func fetchStatus(ctx context.Context, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := noKeepAlive.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, nil
}

// readsBody stands in for a handler that reads the body and reports a read error the way handlers do.
var readsBody = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if _, err := io.ReadAll(r.Body); err != nil {
		writeError(w, r, discard, err)
		return
	}
	w.WriteHeader(http.StatusOK)
})

func postRec(t *testing.T, h http.Handler, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://app.test/api/x", bytes.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func post(t *testing.T, h http.Handler, body []byte, headers map[string]string) int {
	t.Helper()
	return postRec(t, h, body, headers).Code
}

var sameOrigin = map[string]string{"Sec-Fetch-Site": "same-origin"}

func TestBodyTooLarge(t *testing.T) {
	h := protect(Deps{Logger: discard}, readsBody)
	if code := post(t, h, make([]byte, maxBodyBytes), sameOrigin); code != http.StatusOK {
		t.Fatalf("body at the limit: code = %d, want 200", code)
	}
	assertProblem(t, postRec(t, h, make([]byte, maxBodyBytes+1), sameOrigin), http.StatusRequestEntityTooLarge,
		"/problems/payload-too-large")
}

func TestCrossOriginPostRejected(t *testing.T) {
	h := protect(Deps{Logger: discard}, readsBody)
	for _, tc := range []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"cross-site", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"same-site sibling", map[string]string{"Sec-Fetch-Site": "same-site"}, http.StatusForbidden},
		{"foreign Origin without Sec-Fetch-Site", map[string]string{"Origin": "https://evil.test"}, http.StatusForbidden},
		{"same-origin", map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusOK},
		{"non-browser client", nil, http.StatusOK},
	} {
		rec := postRec(t, h, []byte("{}"), tc.headers)
		if tc.want == http.StatusForbidden {
			if p := assertProblem(t, rec, http.StatusForbidden, "/problems/forbidden"); p.Code == nil || *p.Code != codeCrossOrigin {
				t.Errorf("%s: code = %v, want %s", tc.name, p.Code, codeCrossOrigin)
			}
			continue
		}
		if rec.Code != tc.want {
			t.Errorf("%s: code = %d, want %d", tc.name, rec.Code, tc.want)
		}
	}
}

func TestSlowHeaderClosed(t *testing.T) {
	h := newTestHandler(t, ok)
	timeouts := DefaultTimeouts
	timeouts.ReadHeader = 100 * time.Millisecond
	srv := NewServer(h, timeouts)
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	// Send half a header and stall (slowloris).
	if _, err := io.WriteString(conn, "GET /health HTTP/1.1\r\nHost: app.test\r\n"); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(conn); err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			t.Fatal("server kept the connection open past ReadHeaderTimeout")
		}
	}
}

func TestUnknownPathNotFound(t *testing.T) {
	h := newTestHandler(t, ok)
	for _, path := range []string{"/nope", "/health/", "/api/nope", "/api/"} {
		assertProblem(t, get(t, h, path), http.StatusNotFound, "/problems/not-found")
	}
}
