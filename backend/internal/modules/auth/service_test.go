package auth

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Will413028/fullstack-template/backend/internal/platform/apperr"
)

// cheapArgon2 keeps tests fast, under the race detector too; production uses DefaultArgon2Params.
var cheapArgon2 = Argon2Params{MemoryKiB: 64, Time: 1, Threads: 1}

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestService(t *testing.T) (*Service, *testClock, *pgxpool.Pool) {
	t.Helper()
	db := pg.NewMigratedDB(t)
	pool, err := pgxpool.New(t.Context(), db.AppURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	clock := &testClock{t: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}
	svc, err := New(t.Context(), pool, Config{
		SessionAbsolute: 7 * 24 * time.Hour, SessionIdle: 24 * time.Hour,
		Hasher: NewArgon2Hasher(cheapArgon2, 0), Now: clock.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, clock, pool
}

func TestArgon2Hash(t *testing.T) {
	h := NewArgon2Hasher(DefaultArgon2Params, 0)
	encoded, err := h.Hash(t.Context(), "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("hash %q does not carry OWASP's argon2id parameters", encoded)
	}
	for pw, want := range map[string]bool{"correct horse battery staple": true, "correct horse battery stapler": false} {
		if ok, err := h.Verify(t.Context(), pw, encoded); err != nil || ok != want {
			t.Errorf("Verify(%q) = %v, %v; want %v", pw, ok, err, want)
		}
	}
	// Hashes made with other parameters keep verifying.
	other, err := NewArgon2Hasher(cheapArgon2, 0).Hash(t.Context(), "pw")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := h.Verify(t.Context(), "pw", other); err != nil || !ok {
		t.Errorf("Verify with stored parameters = %v, %v; want true", ok, err)
	}
	for _, bad := range []string{"", "$2b$12$abc", "$argon2id$v=19$m=0,t=1,p=1$c2FsdA$a2V5", "$argon2id$v=19$m=64,t=1,p=1$!$a2V5"} {
		if ok, err := h.Verify(t.Context(), "pw", bad); ok || err != nil {
			t.Errorf("Verify against %q = %v, %v; want a mismatch", bad, ok, err)
		}
	}
}

// Each argon2 hash holds 19 MiB; without a bound, a burst of sign-ins would hold that much per request.
func TestArgonConcurrencyBounded(t *testing.T) {
	h := NewArgon2Hasher(cheapArgon2, 2)
	var running, peak atomic.Int32
	h.key = func(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		sleep(20)
		running.Add(-1)
		return make([]byte, keyLen)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := h.Hash(t.Context(), "password"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if p := peak.Load(); p != 2 {
		t.Fatalf("peak concurrent hashes = %d, want 2", p)
	}
	// A waiting hash gives up with its context.
	ctx, cancel := context.WithCancel(t.Context())
	h.sem <- struct{}{}
	h.sem <- struct{}{}
	cancel()
	if _, err := h.Hash(ctx, "password"); err == nil {
		t.Fatal("Hash with a cancelled context waited for a slot")
	}
}

func sleep(ms int) { time.Sleep(time.Duration(ms) * time.Millisecond) }

// The wait after the n-th consecutive failure (n >= 5) is 2^(n-5) minutes, at most an hour; under 5 failures there
// is none. A blocked attempt does not count, and each attempt is made once the previous wait has passed.
func TestLoginThrottleSchedule(t *testing.T) {
	svc, clock, _ := newTestService(t)
	for n := 1; n <= 12; n++ {
		if _, _, err := svc.SignIn(t.Context(), "nobody", "wrong password", "", client); !isCode(err, CodeInvalidCredentials) {
			t.Fatalf("attempt %d: err = %v, want invalid credentials", n, err)
		}
		if n < 5 {
			continue
		}
		want := min(time.Duration(1<<(n-5))*time.Minute, time.Hour)
		_, _, err := svc.SignIn(t.Context(), "nobody", "wrong password", "", client)
		e, ok := apperr.As(err)
		if !ok || e.Code != CodeLoginThrottled || e.RetryAfter != want {
			t.Fatalf("after %d failures: err = %v, want a %s wait", n, err, want)
		}
		clock.Advance(want)
	}
}

func isCode(err error, code string) bool {
	e, ok := apperr.As(err)
	return ok && e.Code == code
}

// client is where the service tests sign in from.
var client = Client{Address: "192.0.2.1"}

func TestCleanup(t *testing.T) {
	svc, clock, pool := newTestService(t)
	ctx := t.Context()
	if _, _, err := svc.CreateUser(ctx, "idle", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	_, _, _ = svc.SignIn(ctx, "ghost", "wrong password", "", client)
	clock.Advance(25 * time.Hour)
	if _, _, err := svc.CreateUser(ctx, "fresh", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	_, _, _ = svc.SignIn(ctx, "ghost2", "wrong password", "", client)
	n, err := svc.Cleanup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// a failed sign-in keeps two rows: its client's and the account-wide count
	if n.Sessions != 1 || n.Throttles != 2 {
		t.Errorf("deleted %d sessions and %d throttles, want 1 and 2 (the idle ones)", n.Sessions, n.Throttles)
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&left); err != nil || left != 1 {
		t.Errorf("sessions left = %d (%v), want 1", left, err)
	}
	// a trusted device lasts 180 days
	if _, _, err := svc.SignIn(ctx, "fresh", "a long enough password", "", client); err != nil {
		t.Fatal(err)
	}
	clock.Advance(179 * 24 * time.Hour)
	if n, err := svc.Cleanup(ctx); err != nil || n.Devices != 0 {
		t.Errorf("after 179 days: deleted %d devices (%v), want 0", n.Devices, err)
	}
	clock.Advance(2 * 24 * time.Hour)
	if n, err := svc.Cleanup(ctx); err != nil || n.Devices != 1 {
		t.Errorf("after 181 days: deleted %d devices (%v), want 1", n.Devices, err)
	}
}

// last_used_at is written at most once a minute, but a session in use must not lapse under a shorter idle limit.
func TestShortIdleLimitKeepsUsedSession(t *testing.T) {
	db := pg.NewMigratedDB(t)
	pool, err := pgxpool.New(t.Context(), db.AppURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	clock := &testClock{t: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}
	svc, err := New(t.Context(), pool, Config{SessionAbsolute: time.Hour, SessionIdle: 30 * time.Second,
		Hasher: NewArgon2Hasher(cheapArgon2, 0), Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	_, sess, err := svc.CreateUser(t.Context(), "alice", "a long enough password")
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		clock.Advance(20 * time.Second)
		if _, _, err := svc.Authenticate(t.Context(), sess.Token); err != nil {
			t.Fatalf("session used every 20s with a 30s idle limit: %v", err)
		}
	}
}
