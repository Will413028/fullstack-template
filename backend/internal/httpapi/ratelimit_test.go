package httpapi

import (
	"context"
	"net/http"
	"net/netip"
	"testing"
	"time"
)

// newLimitedAPI is the full chain with POST /api/widgets (createWidget) limited to perMin per client address and
// GET /api/widgets (listWidgets) not limited. The stub operations run the strict middleware the way the generated
// strict handler does: an error goes to writeError.
func newLimitedAPI(t *testing.T, perMin int) http.Handler {
	t.Helper()
	mw := limitOperations(newIPLimiter(perMin, time.Now), "createWidget")
	stub := func(context.Context, http.ResponseWriter, *http.Request, any) (any, error) { return nil, nil }
	operation := func(id string, code int) http.HandlerFunc {
		f := mw(stub, id)
		return func(w http.ResponseWriter, r *http.Request) {
			if _, err := f(r.Context(), w, r, nil); err != nil {
				writeError(w, r, discard, err)
				return
			}
			w.WriteHeader(code)
		}
	}
	routes := http.NewServeMux()
	routes.Handle("POST /api/widgets", operation("createWidget", http.StatusCreated))
	routes.Handle("GET /api/widgets", operation("listWidgets", http.StatusOK))
	return assemble(Deps{Ready: ok, Logger: discard, Now: time.Now, TrustedProxies: proxies}, loadTestSpec(t), routes)
}

func postFrom(t *testing.T, h http.Handler, remote, xff string) int {
	t.Helper()
	headers := map[string]string{}
	if xff != "" {
		headers["X-Forwarded-For"] = xff
	}
	return doFrom(t, h, http.MethodPost, "/api/widgets", remote, headers).Code
}

func TestRateLimitPerIPBehindTrustedProxy(t *testing.T) {
	h := newLimitedAPI(t, 2)
	for i := range 2 {
		if code := postFrom(t, h, proxyAddr, "198.51.100.1"); code != http.StatusCreated {
			t.Fatalf("request %d from client 1 = %d, want 201", i+1, code)
		}
	}
	rec := doFrom(t, h, http.MethodPost, "/api/widgets", proxyAddr, map[string]string{"X-Forwarded-For": "198.51.100.1"})
	assertProblem(t, rec, http.StatusTooManyRequests, "/problems/rate-limited")
	if ra := rec.Header().Get("Retry-After"); ra != "30" {
		t.Errorf("Retry-After = %q, want 30 (one token every 30s at 2/min)", ra)
	}
	// A second client behind the same proxy has its own bucket.
	if code := postFrom(t, h, proxyAddr, "198.51.100.2"); code != http.StatusCreated {
		t.Fatalf("client 2 = %d, want 201", code)
	}
}

func TestRateLimitIgnoresXFFFromUntrustedPeer(t *testing.T) {
	h := newLimitedAPI(t, 2)
	postFrom(t, h, untrustedAddr, "198.51.100.1")
	postFrom(t, h, untrustedAddr, "198.51.100.2")
	// A forged X-Forwarded-For must not buy a fresh bucket.
	if code := postFrom(t, h, untrustedAddr, "198.51.100.3"); code != http.StatusTooManyRequests {
		t.Fatalf("third request with a new XFF from the same peer = %d, want 429", code)
	}
}

func TestRateLimitOnlyNamedOperations(t *testing.T) {
	h := newLimitedAPI(t, 1)
	postFrom(t, h, untrustedAddr, "")
	for range 3 {
		if code := doFrom(t, h, http.MethodGet, "/api/widgets", untrustedAddr, nil).Code; code != http.StatusOK {
			t.Fatalf("unlimited operation = %d, want 200", code)
		}
	}
}

func TestIPLimiterRefillsAndSweeps(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	now := start
	l := newIPLimiter(2, func() time.Time { return now })
	a, b, c := netip.MustParseAddr("198.51.100.1"), netip.MustParseAddr("198.51.100.2"), netip.MustParseAddr("198.51.100.3")
	l.wait(a)
	l.wait(a)
	if d := l.wait(a); d != 30*time.Second {
		t.Fatalf("third wait = %s, want 30s", d)
	}
	now = start.Add(30 * time.Second)
	if d := l.wait(a); d != 0 {
		t.Fatalf("after 30s wait = %s, want a token", d)
	}
	// Sweeps run at most once a refill; this one, at 60s, finds nothing idle long enough.
	now = start.Add(60 * time.Second)
	l.wait(b)
	// The sweep at 120s: a has been idle over a refill (its bucket is full again), b only 20s.
	now = start.Add(100 * time.Second)
	l.wait(b)
	now = start.Add(120 * time.Second)
	l.wait(c)
	_, keptA := l.buckets[a]
	_, keptB := l.buckets[b]
	if keptA || !keptB {
		t.Fatalf("after sweep: a kept = %v, b kept = %v; want only the idle a dropped", keptA, keptB)
	}
}

func TestIPLimiterKeysIPv6By64(t *testing.T) {
	l := newIPLimiter(1, time.Now)
	l.wait(netip.MustParseAddr("2001:db8:1:2::1"))
	if d := l.wait(netip.MustParseAddr("2001:db8:1:2:ffff::9")); d == 0 {
		t.Fatal("another address in the same /64 got a fresh bucket")
	}
	if d := l.wait(netip.MustParseAddr("2001:db8:1:3::1")); d != 0 {
		t.Fatalf("a different /64 waited %s, want its own bucket", d)
	}
}
