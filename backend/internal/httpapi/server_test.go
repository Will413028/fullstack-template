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

func newTestHandler(ping func(context.Context) error) (http.Handler, *Readiness) {
	ready := NewReadiness(ping)
	return NewHandler(Deps{Ready: ready, Logger: discard}), ready
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
	h, _ := newTestHandler(func(context.Context) error { return errors.New("db down") })
	// Liveness ignores dependencies: still 200 with the database down.
	assertStatusBody(t, get(t, h, "/health"), http.StatusOK, "ok")
}

func TestReady(t *testing.T) {
	h, _ := newTestHandler(ok)
	assertStatusBody(t, get(t, h, "/health/ready"), http.StatusOK, "ready")
}

func TestReadyDBDown(t *testing.T) {
	h, _ := newTestHandler(func(context.Context) error { return errors.New("db down") })
	if rec := get(t, h, "/health/ready"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
}

func TestReadyDuringShutdown(t *testing.T) {
	h, ready := newTestHandler(ok)
	srv := NewServer(h, DefaultTimeouts)
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, srv, ready, 500*time.Millisecond, 5*time.Second, discard) }()

	url := "http://" + ln.Addr().String() + "/health/ready"
	if code := status(t, url); code != http.StatusOK {
		t.Fatalf("before shutdown: code = %d, want 200", code)
	}
	cancel() // what signal.NotifyContext does on SIGTERM
	deadline := time.Now().Add(400 * time.Millisecond)
	for status(t, url) != http.StatusServiceUnavailable {
		if time.Now().After(deadline) {
			t.Fatal("readiness did not turn 503 during the drain delay")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after drain and shutdown")
	}
}

// noKeepAlive dials a fresh connection per request. The default client's pool can leave a spare connection that
// never sends a request; Shutdown treats such a StateNew connection as active for 5s, which made the test flaky.
var noKeepAlive = &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

func status(t *testing.T, url string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := noKeepAlive.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

// readsBody stands in for a handler that reads the body; it answers 413 when the body exceeds the limit.
var readsBody = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if _, err := io.ReadAll(r.Body); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
})

func post(t *testing.T, h http.Handler, body []byte, headers map[string]string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://app.test/api/x", bytes.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestBodyTooLarge(t *testing.T) {
	h := protect(readsBody)
	sameOrigin := map[string]string{"Sec-Fetch-Site": "same-origin"}
	if code := post(t, h, make([]byte, maxBodyBytes), sameOrigin); code != http.StatusOK {
		t.Fatalf("body at the limit: code = %d, want 200", code)
	}
	if code := post(t, h, make([]byte, maxBodyBytes+1), sameOrigin); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body over the limit: code = %d, want 413", code)
	}
}

func TestCrossOriginPostRejected(t *testing.T) {
	h := protect(readsBody)
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
		if code := post(t, h, []byte("{}"), tc.headers); code != tc.want {
			t.Errorf("%s: code = %d, want %d", tc.name, code, tc.want)
		}
	}
}

func TestSlowHeaderClosed(t *testing.T) {
	h, _ := newTestHandler(ok)
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
	h, _ := newTestHandler(ok)
	if rec := get(t, h, "/nope"); rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
	if rec := get(t, h, "/health/"); rec.Code != http.StatusNotFound {
		t.Fatalf("/health/: code = %d, want 404", rec.Code)
	}
}
