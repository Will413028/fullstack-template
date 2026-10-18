package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthcheck(t *testing.T) {
	code := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health/ready" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(code)
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	if err := healthcheck(t.Context(), addr); err != nil {
		t.Fatalf("ready server: %v", err)
	}
	code = http.StatusServiceUnavailable
	if err := healthcheck(t.Context(), addr); err == nil {
		t.Fatal("503 reported as healthy")
	}
	srv.Close()
	if err := healthcheck(t.Context(), addr); err == nil {
		t.Fatal("closed server reported as healthy")
	}
}

func TestProbeAddr(t *testing.T) {
	for in, want := range map[string]string{":8000": "127.0.0.1:8000", "0.0.0.0:9000": "127.0.0.1:9000", "10.0.0.5:8000": "10.0.0.5:8000"} {
		if got := probeAddr(in); got != want {
			t.Errorf("probeAddr(%q) = %q, want %q", in, got, want)
		}
	}
}
