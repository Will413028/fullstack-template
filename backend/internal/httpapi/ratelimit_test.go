package httpapi

import (
	"net/http"
	"net/netip"
	"testing"
	"time"

	"connectrpc.com/connect"

	testingv1 "github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc/testdata/gen/app/testing/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc/testdata/gen/app/testing/v1/testingv1connect"
)

// newLimitedAPI is NewHandler with the class test services behind the trusted proxies: UpdatePublicThing is limited
// to perMin per client address at the Connect entry, GetPublicThing is not. The clock does not move.
func newLimitedAPI(t *testing.T, perMin int) http.Handler {
	t.Helper()
	clock := &testClock{t: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}
	return newConnectAPI(t, Deps{RateLimitPerMin: perMin, Now: clock.Now, TrustedProxies: proxies,
		testLimited: []string{testingv1connect.TestingPublicServiceUpdatePublicThingProcedure}})
}

// publicThingsFrom is the test Public service's client as the peer remote sees it, with X-Forwarded-For when xff is
// not empty.
func publicThingsFrom(h http.Handler, remote, xff string) testingv1connect.TestingPublicServiceClient {
	opts := []requestOpt{fromPeer(remote)}
	if xff != "" {
		opts = append(opts, withHeader("X-Forwarded-For", xff))
	}
	return testingv1connect.NewTestingPublicServiceClient(&http.Client{Transport: handlerTransport{h: h, opts: opts}}, baseURL)
}

// updateFrom calls the limited UpdatePublicThing.
func updateFrom(t *testing.T, h http.Handler, remote, xff string) error {
	t.Helper()
	_, err := publicThingsFrom(h, remote, xff).UpdatePublicThing(t.Context(), connect.NewRequest(&testingv1.UpdatePublicThingRequest{
		Thing: thingFor("1"),
	}))
	return err
}

func TestRateLimitPerIPBehindTrustedProxy(t *testing.T) {
	h := newLimitedAPI(t, 2)
	for i := range 2 {
		if err := updateFrom(t, h, proxyAddr, "198.51.100.1"); err != nil {
			t.Fatalf("request %d from client 1: %v", i+1, err)
		}
	}
	err := updateFrom(t, h, proxyAddr, "198.51.100.1")
	assertConnectError(t, err, connect.CodeResourceExhausted, "")
	assertRetryInfo(t, err, 30*time.Second) // one token every 30s at 2/min
	// A second client behind the same proxy has its own bucket.
	if err := updateFrom(t, h, proxyAddr, "198.51.100.2"); err != nil {
		t.Fatalf("client 2: %v", err)
	}
}

func TestRateLimitIgnoresXFFFromUntrustedPeer(t *testing.T) {
	h := newLimitedAPI(t, 2)
	for _, xff := range []string{"198.51.100.1", "198.51.100.2"} {
		if err := updateFrom(t, h, untrustedAddr, xff); err != nil {
			t.Fatal(err)
		}
	}
	// A forged X-Forwarded-For must not buy a fresh bucket.
	assertConnectError(t, updateFrom(t, h, untrustedAddr, "198.51.100.3"), connect.CodeResourceExhausted, "")
}

func TestRateLimitOnlyNamedOperations(t *testing.T) {
	h := newLimitedAPI(t, 1)
	if err := updateFrom(t, h, untrustedAddr, ""); err != nil {
		t.Fatal(err)
	}
	assertConnectError(t, updateFrom(t, h, untrustedAddr, ""), connect.CodeResourceExhausted, "")
	for range 3 {
		if _, err := publicThingsFrom(h, untrustedAddr, "").GetPublicThing(t.Context(),
			connect.NewRequest(&testingv1.GetPublicThingRequest{Id: "1"})); err != nil {
			t.Fatalf("unlimited procedure: %v", err)
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
