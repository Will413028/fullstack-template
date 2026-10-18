package hibp

import (
	"crypto/sha1" //nolint:gosec // the range API's key
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// rangeKey splits the uppercase SHA-1 hex of s the way the range API does.
func rangeKey(s string) (prefix, suffix string) {
	sum := sha1.Sum([]byte(s)) //nolint:gosec // see the import
	digest := strings.ToUpper(hex.EncodeToString(sum[:]))
	return digest[:5], digest[5:]
}

func TestBreached(t *testing.T) {
	const phrase = "correct horse battery staple"
	prefix, suffix := rangeKey(phrase)
	var gotPath, gotPadding string
	body := suffix + ":42\r\n0000000000000000000000000000000000A:0\r\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotPadding = r.URL.Path, r.Header.Get("Add-Padding")
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	breached, err := c.Breached(t.Context(), phrase)
	if err != nil || !breached {
		t.Fatalf("Breached = %v, %v; want true", breached, err)
	}
	if gotPath != "/range/"+prefix || gotPadding != "true" {
		t.Errorf("request path %q, Add-Padding %q; want /range/%s and true (only the prefix leaves)", gotPath, gotPadding, prefix)
	}
	body = suffix + ":0\r\n"
	if breached, err := c.Breached(t.Context(), phrase); err != nil || breached {
		t.Errorf("padding entry (count 0) = %v, %v; want false", breached, err)
	}
	body = ""
	if breached, err := c.Breached(t.Context(), phrase); err != nil || breached {
		t.Errorf("suffix absent = %v, %v; want false", breached, err)
	}
}

func TestBreachedServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	if _, err := c.Breached(t.Context(), "correct horse battery staple"); err == nil {
		t.Fatal("503 reported as a result")
	}
}
