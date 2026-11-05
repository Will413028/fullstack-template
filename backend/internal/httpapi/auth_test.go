package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	adminusersv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/admin/users/v1"
	sessionsv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/sessions/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/gen/app/sessions/v1/sessionsv1connect"
	usersv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/users/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/gen/app/users/v1/usersv1connect"
	"github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc"
	"github.com/Will413028/template-go-nextjs/backend/internal/modules/auth"
	"github.com/Will413028/template-go-nextjs/backend/internal/modules/items"
	platformdb "github.com/Will413028/template-go-nextjs/backend/internal/platform/db"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/dbtest"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/logx"
)

var pg *dbtest.Server

func TestMain(m *testing.M) { dbtest.Main(m, &pg) }

const (
	cookieName   = "__Host-session"
	goodPassword = "a long enough password"
)

// cheapArgon2 keeps the tests fast under the race detector; production uses auth.DefaultArgon2Params.
var cheapArgon2 = auth.Argon2Params{MemoryKiB: 64, Time: 1, Threads: 1}

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

// countingHasher counts Verify calls and can slow every hash down.
type countingHasher struct {
	auth.PasswordHasher
	verifies atomic.Int32
	delay    time.Duration
}

func (h *countingHasher) Verify(ctx context.Context, password, encoded string) (bool, error) {
	h.verifies.Add(1)
	time.Sleep(h.delay)
	return h.PasswordHasher.Verify(ctx, password, encoded)
}

type breachFunc func(context.Context, string) (bool, error)

func (f breachFunc) Breached(ctx context.Context, p string) (bool, error) { return f(ctx, p) }

// authAPI is the real handler, with the auth and items services, over a migrated test database. Its generated
// clients are in auth_client_test.go and items_test.go.
type authAPI struct {
	t      *testing.T
	h      http.Handler
	svc    *auth.Service
	pool   *pgxpool.Pool
	clock  *testClock
	hasher *countingHasher
	// queries counts the statements sent to the database.
	queries *queryCounter
}

// queryCounter is a pgx tracer that counts queries.
type queryCounter struct{ n atomic.Int64 }

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.n.Add(1)
	return ctx
}

func (c *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// countQueries returns how many queries the call send makes sends.
func (a *authAPI) countQueries(send func() error) int64 {
	a.t.Helper()
	before := a.queries.n.Load()
	if err := send(); err != nil {
		a.t.Fatal(err)
	}
	return a.queries.n.Load() - before
}

type authOption func(*auth.Config, *Deps)

func newAuthAPI(t *testing.T, opts ...authOption) *authAPI {
	t.Helper()
	db := pg.NewMigratedDB(t)
	// opened the way serve opens it (TimeZone=UTC, timeouts)
	queries := &queryCounter{}
	pool, err := platformdb.Open(t.Context(), platformdb.Config{
		URL: db.AppURL, Password: dbtest.AppRole, MaxConns: 10, StatementTimeout: 5 * time.Second, Tracer: queries,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	a := &authAPI{t: t, pool: pool, queries: queries,
		clock:  &testClock{t: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)},
		hasher: &countingHasher{PasswordHasher: auth.NewArgon2Hasher(cheapArgon2, 0)},
	}
	cfg := auth.Config{SessionAbsolute: 7 * 24 * time.Hour, SessionIdle: 24 * time.Hour, Hasher: a.hasher, Now: a.clock.Now}
	deps := Deps{Ready: ok, Logger: discard, Now: a.clock.Now, RateLimitPerMin: 1000,
		Cookie: Cookie{Name: cookieName, DeviceName: deviceCookieName, Secure: true}, Items: items.New(pool)}
	for _, o := range opts {
		o(&cfg, &deps)
	}
	if a.svc, err = auth.New(t.Context(), pool, cfg); err != nil {
		t.Fatal(err)
	}
	deps.Auth = a.svc
	a.h = newTestHandlerDeps(t, deps)
	return a
}

// assertMe checks whether token is a valid session: GetCurrentUser succeeds, or is unauthenticated (SESSION_INVALID).
func assertMe(t *testing.T, a *authAPI, token string, valid bool) {
	t.Helper()
	_, err := a.me(token)
	if valid {
		if err != nil {
			t.Fatalf("GetCurrentUser = %v, want the user", err)
		}
		return
	}
	assertConnectError(t, err, connect.CodeUnauthenticated, "SESSION_INVALID")
}

func TestCreateUserSetsSessionCookie(t *testing.T) {
	a := newAuthAPI(t)
	resp, err := a.signUp("Alice", goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	u := resp.Msg.GetUser()
	if u.GetAccount() != "alice" || u.GetRole() != usersv1.Role_ROLE_USER || u.Disabled == nil || u.GetDisabled() {
		t.Errorf("user = %v, want a lowercase, enabled user", u)
	}
	if id, err := uuid.Parse(u.GetId()); err != nil || id.Version() != 7 || !u.GetCreateTime().IsValid() {
		t.Errorf("user = %v, want a UUIDv7 id and a create_time", u)
	}
	c := a.sessionCookie(resp.Header())
	if !c.HttpOnly || !c.Secure || c.Path != "/" || c.SameSite != http.SameSiteLaxMode || c.Domain != "" {
		t.Errorf("cookie %q lacks HttpOnly, Secure, Path=/ or SameSite=Lax, or has a Domain", resp.Header().Values("Set-Cookie"))
	}
	if c.MaxAge != 7*24*3600 {
		t.Errorf("Max-Age = %d, want the 7-day absolute lifetime", c.MaxAge)
	}
	if len(c.Value) != 43 {
		t.Errorf("token %q is not 32 bytes of base64url", c.Value)
	}
	// the new user reads back with the session the response set (followLocation checked this for the 201's Location)
	if got, err := a.getUser(c.Value, u.GetId()); err != nil || !proto.Equal(got, u) {
		t.Errorf("GetUser with the new session = %v, %v; want %v", got, err, u)
	}
}

func TestCreateUserDuplicate(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	_, err := a.signUp("ALICE", goodPassword)
	assertConnectError(t, err, connect.CodeAlreadyExists, "ACCOUNT_TAKEN")
}

// Concurrent sign-ups for one account: the unique index decides, so exactly one gets the account and the rest
// already_exists.
func TestCreateUserConcurrentSameAccount(t *testing.T) {
	a := newAuthAPI(t)
	const n = 20
	outcomes := make(chan string, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			_, err := a.signUp("alice", goodPassword)
			outcomes <- outcome(err)
		})
	}
	wg.Wait()
	close(outcomes)
	got := map[string]int{}
	for o := range outcomes {
		got[o]++
	}
	if got["ok"] != 1 || got[connect.CodeAlreadyExists.String()] != n-1 {
		t.Errorf("outcomes = %v, want one ok and %d already_exists", got, n-1)
	}
}

// The account and password limits of a sign-up (ADR api-contract-protobuf-connect D2: the auth service checks them, the
// same for seed-admin): account 3 to 50 of [A-Za-z0-9_.-], password 15 to 128 code points. Every invalid field is
// reported in one response.
func TestCreateUserLimits(t *testing.T) {
	a := newAuthAPI(t)
	for _, tc := range []struct {
		account, password string
		want              []string
	}{
		{"alice", "fourteen chars", []string{"password TOO_SHORT"}},
		{"alice", strings.Repeat("p", 129), []string{"password TOO_LONG"}},
		{"alice", "", []string{"password REQUIRED"}},
		{"ab", goodPassword, []string{"account TOO_SHORT"}},
		{strings.Repeat("a", 51), goodPassword, []string{"account TOO_LONG"}},
		{"al ice", goodPassword, []string{"account PATTERN"}},
		// was too_short (the contract's minLength); proto3 cannot tell empty from unset, so both are REQUIRED
		{"", goodPassword, []string{"account REQUIRED"}},
		{"ab", "short", []string{"account TOO_SHORT", "password TOO_SHORT"}},
		{"", "", []string{"account REQUIRED", "password REQUIRED"}},
	} {
		_, err := a.signUp(tc.account, tc.password)
		assertViolations(t, err, tc.want...)
	}
	// the longest account, and the longest password counted in code points (384 bytes)
	if _, err := a.signUp(strings.Repeat("a", 50), strings.Repeat("字", 128)); err != nil {
		t.Errorf("50-character account, 128-character password: %v", err)
	}
	if _, err := a.signUp("bob", "fifteen chars!!"); err != nil {
		t.Errorf("15-character password: %v", err)
	}
	// Intended change (the move to Connect): a field the request does not have is dropped (connect-go's DiscardUnknown),
	// no longer 422 additional_property; it cannot choose the role either.
	rec := connectPost(t, a.h, usersv1connect.UserPublicServiceCreateUserProcedure,
		`{"account":"eve","password":"`+goodPassword+`","role":"ROLE_ADMIN","isAdmin":true}`, nil)
	var created usersv1.CreateUserResponse
	if err := protojson.Unmarshal(rec.Body.Bytes(), &created); err != nil || rec.Code != http.StatusOK ||
		created.GetUser().GetRole() != usersv1.Role_ROLE_USER {
		t.Errorf("sign-up with unknown fields = %d %s, want a user with role user", rec.Code, rec.Body)
	}
}

// NIST SP 800-63B-4 forbids composition rules: a long all-lowercase password is fine.
func TestCreateUserNoCompositionRule(t *testing.T) {
	a := newAuthAPI(t)
	if _, err := a.signUp("alice", "onlylowercaseletters"); err != nil {
		t.Fatal(err)
	}
}

func TestCreateUserCommonPassword(t *testing.T) {
	a := newAuthAPI(t)
	// on the embedded list (SecLists Pwdb), 15 characters
	_, err := a.signUp("alice", "demon1q2w3e4r5t")
	assertFieldViolation(t, err, "password", "PASSWORD_TOO_COMMON")
}

func TestCreateUserBreachedPassword(t *testing.T) {
	var checked string
	breached := true
	var failing error
	a := newAuthAPI(t, func(c *auth.Config, _ *Deps) {
		c.Breach = breachFunc(func(_ context.Context, p string) (bool, error) { checked = p; return breached, failing })
	})
	// The check sees the normalized password (NFKC folds the full-width letters).
	_, err := a.signUp("alice", "ｂｒｅａｃｈｅｄ ｐａｓｓｐｈｒａｓｅ")
	assertFieldViolation(t, err, "password", "PASSWORD_TOO_COMMON")
	if checked != "breached passphrase" {
		t.Errorf("breach check got %q, want the NFKC form", checked)
	}
	// An unreachable breach service does not block sign-ups.
	failing = errors.New("timeout")
	if _, err := a.signUp("alice", goodPassword); err != nil {
		t.Fatalf("with the breach check failing: %v", err)
	}
}

func TestCreateSession(t *testing.T) {
	a := newAuthAPI(t)
	created, _ := a.mustSignUp("alice")
	resp, err := a.signIn("alice", goodPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Msg.GetSession().GetUser().GetId(); got != created.GetId() {
		t.Errorf("signed in as %s, want %s", got, created.GetId())
	}
	assertMe(t, a, a.sessionCookie(resp.Header()).Value, true)
}

// The limits of a sign-in are only presence and the longest account and password that can exist: a rule tightened
// later must not lock out an account created before it. Every invalid field is reported in one response.
func TestCreateSessionLimits(t *testing.T) {
	a := newAuthAPI(t)
	for _, tc := range []struct {
		account, password string
		want              []string
	}{
		{"alice", strings.Repeat("p", 129), []string{"password TOO_LONG"}},
		{strings.Repeat("a", 51), goodPassword, []string{"account TOO_LONG"}},
		{"", "", []string{"account REQUIRED", "password REQUIRED"}},
		{strings.Repeat("a", 51), strings.Repeat("p", 129), []string{"account TOO_LONG", "password TOO_LONG"}},
	} {
		_, err := a.signIn(tc.account, tc.password, "")
		assertViolations(t, err, tc.want...)
	}
	// within the limits, short or long, the credentials decide
	for _, c := range [][2]string{{"ab", "x"}, {strings.Repeat("a", 50), strings.Repeat("字", 128)}} {
		_, err := a.signIn(c[0], c[1], "")
		assertConnectError(t, err, connect.CodeUnauthenticated, "INVALID_CREDENTIALS")
	}
}

func TestCreateSessionWrongPassword(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	_, err := a.signIn("alice", "not the password", "")
	assertConnectError(t, err, connect.CodeUnauthenticated, "INVALID_CREDENTIALS")
}

// An unknown account costs one password verification, like a wrong password, and gets the same response.
func TestCreateSessionUnknownAccountRunsHash(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	_, wrong := a.signIn("alice", "not the password", "")
	a.hasher.verifies.Store(0)
	_, unknown := a.signIn("bob", "not the password", "")
	if n := a.hasher.verifies.Load(); n != 1 {
		t.Errorf("verifications for an unknown account = %d, want 1", n)
	}
	assertConnectError(t, unknown, connect.CodeUnauthenticated, "INVALID_CREDENTIALS")
	assertSameError(t, unknown, wrong)
}

// Signing in always issues a new token (no session fixation) and ends the session the request carried.
func TestCreateSessionIssuesNewToken(t *testing.T) {
	a := newAuthAPI(t)
	_, old := a.mustSignUp("alice")
	resp, err := a.signIn("alice", goodPassword, old)
	if err != nil {
		t.Fatal(err)
	}
	fresh := a.sessionCookie(resp.Header()).Value
	if fresh == old {
		t.Fatal("sign-in kept the token the request carried")
	}
	assertMe(t, a, fresh, true)
	assertMe(t, a, old, false)
}

func TestCreateSessionMixedCaseAccount(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	a.mustSignIn("ALICE", goodPassword)
}

// The same password typed with full-width or half-width characters is the same password (NFKC, NIST SP 800-63B-4).
func TestPasswordNormalized(t *testing.T) {
	a := newAuthAPI(t)
	if _, err := a.signUp("alice", "ｃｏｒｒｅｃｔ－ｈｏｒｓｅ－ｂａｔｔｅｒｙ"); err != nil {
		t.Fatalf("sign up: %v", err)
	}
	a.mustSignIn("alice", "correct-horse-battery")
	if _, err := a.signUp("bob", "correct-horse-battery-staple"); err != nil {
		t.Fatalf("sign up: %v", err)
	}
	a.mustSignIn("bob", "ｃｏｒｒｅｃｔ－ｈｏｒｓｅ－ｂａｔｔｅｒｙ－ｓｔａｐｌｅ")
}

func TestCreateSessionDisabledAccount(t *testing.T) {
	a := newAuthAPI(t)
	u, _ := a.mustSignUp("alice")
	if _, err := a.svc.SetDisabled(t.Context(), uuid.MustParse(u.GetId()), true); err != nil {
		t.Fatal(err)
	}
	// the same answer as a wrong password: a disabled account is not told apart
	_, err := a.signIn("alice", goodPassword, "")
	assertConnectError(t, err, connect.CodeUnauthenticated, "INVALID_CREDENTIALS")
}

func TestMe(t *testing.T) {
	a := newAuthAPI(t)
	created, token := a.mustSignUp("alice")
	u, err := a.me(token)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := uuid.Parse(u.GetId()); err != nil || id.Version() != 7 || !proto.Equal(u, created) {
		t.Errorf("user = %v, want %v with a UUIDv7 id", u, created)
	}
	if u.GetRole() != usersv1.Role_ROLE_USER || u.GetAccount() != "alice" {
		t.Errorf("user = %v, want account alice with role user", u)
	}
}

func TestMeWithoutCookie(t *testing.T) {
	assertMe(t, newAuthAPI(t), "", false)
}

func TestMeWithUnknownToken(t *testing.T) {
	assertMe(t, newAuthAPI(t), "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", false)
}

// The cookie is HttpOnly, so only the server can remove a dead one; the frontend takes a present cookie for a
// session. Every response to a request with a dead cookie clears it, unless the response signs in: then it carries
// only the new session (clearStaleCookie).
func TestInvalidSessionCookieCleared(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	if _, err := a.signOut(token); err != nil {
		t.Fatal(err)
	}
	_, err := a.me(token)
	assertConnectError(t, err, connect.CodeUnauthenticated, "SESSION_INVALID")
	if c := a.sessionCookie(asConnectError(t, err).Meta()); c.MaxAge >= 0 {
		t.Errorf("Set-Cookie = %q, want the cookie cleared", asConnectError(t, err).Meta().Values("Set-Cookie"))
	}
	// a Public call that does not sign in clears it too, on its error
	a.mustSignUp("bob")
	_, err = a.signIn("bob", "not the password", token)
	assertConnectError(t, err, connect.CodeUnauthenticated, "INVALID_CREDENTIALS")
	if c := a.sessionCookie(asConnectError(t, err).Meta()); c.MaxAge >= 0 {
		t.Errorf("failed sign-in: Set-Cookie = %q, want the cookie cleared", asConnectError(t, err).Meta().Values("Set-Cookie"))
	}
	// signing in or up with the dead cookie gets only the new one
	resp, err := a.signIn("bob", goodPassword, token)
	if err != nil {
		t.Fatal(err)
	}
	if c := a.sessionCookie(resp.Header()); c.MaxAge <= 0 || c.Value == "" {
		t.Errorf("sign-in: Set-Cookie = %q, want only the new session", resp.Header().Values("Set-Cookie"))
	}
	signUp, err := a.publicUsers(token).CreateUser(t.Context(), connect.NewRequest(&usersv1.CreateUserRequest{
		Account: "dave", Password: goodPassword,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c := a.sessionCookie(signUp.Header()); c.MaxAge <= 0 || c.Value == "" {
		t.Errorf("sign-up: Set-Cookie = %q, want only the new session", signUp.Header().Values("Set-Cookie"))
	}
	// a valid session is left alone
	_, live := a.mustSignUp("carol")
	me, err := a.users(live).GetCurrentUser(t.Context(), connect.NewRequest(&usersv1.GetCurrentUserRequest{}))
	if err != nil {
		t.Fatalf("valid session: %v", err)
	}
	if set := me.Header().Values("Set-Cookie"); len(set) > 0 {
		t.Errorf("valid session got Set-Cookie %q", set)
	}
}

// Only the cookie authenticates; a token in Authorization is ignored.
func TestMeIgnoresBearer(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	_, err := a.users("", withHeader("Authorization", "Bearer "+token)).
		GetCurrentUser(t.Context(), connect.NewRequest(&usersv1.GetCurrentUserRequest{}))
	assertConnectError(t, err, connect.CodeUnauthenticated, "SESSION_INVALID")
}

func TestMeDisabledAccount(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	// disabled without going through SetDisabled, so the session row still exists
	if _, err := a.pool.Exec(t.Context(), `UPDATE users SET is_disabled = true`); err != nil {
		t.Fatal(err)
	}
	assertMe(t, a, token, false)
}

func TestMeAfterUserDeleted(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	if _, err := a.pool.Exec(t.Context(), `DELETE FROM users`); err != nil {
		t.Fatal(err)
	}
	assertMe(t, a, token, false)
}

func TestSessionIdleExpiry(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	a.clock.Advance(24*time.Hour - time.Second)
	assertMe(t, a, token, true) // using it resets the idle limit
	a.clock.Advance(24*time.Hour - time.Second)
	assertMe(t, a, token, true)
	a.clock.Advance(24 * time.Hour)
	assertMe(t, a, token, false)
}

func TestSessionAbsoluteExpiry(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	for range 13 {
		a.clock.Advance(12 * time.Hour)
		assertMe(t, a, token, true)
	}
	a.clock.Advance(12 * time.Hour) // 7 days after sign-up, used every 12 hours
	assertMe(t, a, token, false)
}

// Signing out succeeds and clears the cookie whatever session the request carries: a valid one, one that has ended or
// expired, an unknown one, or none. DeleteCurrentSession is a Public method for this: a stale cookie must be able to
// sign out.
func TestDeleteCurrentSessionClearsCookie(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	_, idle := a.mustSignUp("bob")
	assertSignsOut := func(what, tok string) {
		t.Helper()
		resp, err := a.signOut(tok)
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		set := resp.Header().Values("Set-Cookie")
		if c := a.sessionCookie(resp.Header()); c.MaxAge >= 0 || !strings.Contains(strings.Join(set, "\n"), "Max-Age=0") {
			t.Errorf("%s: Set-Cookie = %q, want Max-Age=0", what, set)
		}
	}
	assertSignsOut("valid session", token)
	assertSignsOut("ended session", token)
	assertSignsOut("no session", "")
	assertSignsOut("unknown token", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	a.clock.Advance(25 * time.Hour) // over the 1-day idle limit
	assertSignsOut("expired session", idle)
}

func TestDeleteCurrentSessionRevokes(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	other := a.mustSignIn("alice", goodPassword)
	if _, err := a.signOut(token); err != nil {
		t.Fatal(err)
	}
	assertMe(t, a, token, false)
	assertMe(t, a, other, true)
}

func TestDeleteAllSessions(t *testing.T) {
	a := newAuthAPI(t)
	_, first := a.mustSignUp("alice")
	second := a.mustSignIn("alice", goodPassword)
	_, bob := a.mustSignUp("bob")
	resp, err := a.sessions(second).DeleteAllSessions(t.Context(), connect.NewRequest(&sessionsv1.DeleteAllSessionsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if c := a.sessionCookie(resp.Header()); c.MaxAge >= 0 {
		t.Errorf("cookie not cleared: %q", resp.Header().Values("Set-Cookie"))
	}
	assertMe(t, a, first, false)
	assertMe(t, a, second, false)
	assertMe(t, a, bob, true)
	_, err = a.sessions("").DeleteAllSessions(t.Context(), connect.NewRequest(&sessionsv1.DeleteAllSessionsRequest{}))
	assertConnectError(t, err, connect.CodeUnauthenticated, "SESSION_INVALID")
}

// disable sets user id's disabled through UserAdminService.UpdateUser, with the mask.
func (a *authAPI) disable(token, id string, disabled bool) (*usersv1.User, error) {
	a.t.Helper()
	return a.updateUser(token, &usersv1.User{Id: id, Disabled: proto.Bool(disabled)}, "disabled")
}

// Disabling ends the user's sessions for good: enabling the account again does not bring them back.
func TestAdminDisablesUserRevokesSessions(t *testing.T) {
	a := newAuthAPI(t)
	admin := a.mustAdmin()
	u, token := a.mustSignUp("alice")
	if got, err := a.disable(admin, u.GetId(), true); err != nil || !got.GetDisabled() {
		t.Fatalf("disable = %v, %v", got, err)
	}
	assertMe(t, a, token, false)
	if got, err := a.disable(admin, u.GetId(), false); err != nil || got.Disabled == nil || got.GetDisabled() {
		t.Fatalf("enable = %v, %v", got, err)
	}
	assertMe(t, a, token, false)
	a.mustSignIn("alice", goodPassword)
	// no mask and no disabled changes nothing (was the empty merge patch)
	if got, err := a.updateUser(admin, &usersv1.User{Id: u.GetId()}); err != nil || got.GetDisabled() || got.GetAccount() != "alice" {
		t.Fatalf("empty update = %v, %v", got, err)
	}
	_, err := a.disable(admin, uuid.Must(uuid.NewV7()).String(), true)
	assertConnectError(t, err, connect.CodeNotFound, "")
	_, err = a.disable(admin, "42", true)
	assertConnectError(t, err, connect.CodeNotFound, "")
	_, err = a.updateUser(admin, &usersv1.User{Id: u.GetId()}, "is_disabled")
	assertFieldViolation(t, err, "update_mask", "INVALID")
}

// The only enabled admin cannot be disabled, by themselves or anyone; with a second admin, either may be disabled.
func TestLastAdminCannotBeDisabled(t *testing.T) {
	a := newAuthAPI(t)
	root := a.mustAdmin()
	rootID := a.currentSession(root).GetUser().GetId()
	_, err := a.disable(root, rootID, true)
	assertConnectError(t, err, connect.CodeFailedPrecondition, "LAST_ADMIN")
	if _, err := a.svc.EnsureAdmin(t.Context(), "second", goodPassword); err != nil {
		t.Fatal(err)
	}
	if _, err := a.disable(root, rootID, true); err != nil {
		t.Fatalf("disable self with another admin: %v", err)
	}
	second := a.mustSignIn("second", goodPassword)
	_, err = a.disable(second, a.currentSession(second).GetUser().GetId(), true)
	assertConnectError(t, err, connect.CodeFailedPrecondition, "LAST_ADMIN")
}

// Two admins disabling each other at the same moment: the admin rows are locked first, so exactly one succeeds. The
// other gets failed_precondition, or unauthenticated when its session was already revoked by the winner.
func TestConcurrentAdminDisableLeavesOne(t *testing.T) {
	a := newAuthAPI(t)
	root := a.mustAdmin()
	if _, err := a.svc.EnsureAdmin(t.Context(), "second", goodPassword); err != nil {
		t.Fatal(err)
	}
	second := a.mustSignIn("second", goodPassword)
	targets := map[string]string{root: a.currentSession(second).GetUser().GetId(), second: a.currentSession(root).GetUser().GetId()}
	outcomes := make(chan string, 2)
	var wg sync.WaitGroup
	for token, target := range targets {
		wg.Go(func() {
			_, err := a.disable(token, target, true)
			outcomes <- outcome(err)
		})
	}
	wg.Wait()
	close(outcomes)
	got := map[string]int{}
	for o := range outcomes {
		got[o]++
	}
	if got["ok"] != 1 || got[connect.CodeFailedPrecondition.String()]+got[connect.CodeUnauthenticated.String()] != 1 {
		t.Errorf("outcomes = %v, want one ok and one failed_precondition or unauthenticated", got)
	}
	var enabled int
	if err := a.pool.QueryRow(t.Context(), `SELECT count(*) FROM users WHERE role = 'admin' AND NOT is_disabled`).Scan(&enabled); err != nil || enabled != 1 {
		t.Errorf("enabled admins = %d (%v), want 1", enabled, err)
	}
}

func TestNonAdminCannotPatchUser(t *testing.T) {
	a := newAuthAPI(t)
	victim, victimToken := a.mustSignUp("alice")
	_, token := a.mustSignUp("mallory")
	_, err := a.disable(token, victim.GetId(), true)
	assertConnectError(t, err, connect.CodePermissionDenied, "INSUFFICIENT_ROLE")
	assertMe(t, a, victimToken, true)
}

// Authentication comes before the body (the plan's first invariant): an anonymous caller of any App or Admin method
// gets unauthenticated for a body connect-go cannot decode, or a content type it does not accept, rather than an
// error describing the request it should send. A Public method answers the same body with invalid_argument. No
// database: the check runs before any handler.
func TestUnauthenticatedBeforeValidation(t *testing.T) {
	h := newConnectAPI(t, Deps{})
	for _, s := range connectServices(Deps{}) {
		if class, _ := rpc.ClassOf(s.Desc.FullName()); class == rpc.Public {
			continue
		}
		methods := s.Desc.Methods()
		for i := range methods.Len() {
			procedure := rpc.Procedure(methods.Get(i))
			t.Run(procedure, func(t *testing.T) {
				decodeWireError(t, connectPost(t, h, procedure, `{not json`, nil), http.StatusUnauthorized, "unauthenticated")
				rec := connectPost(t, h, procedure, `{}`, map[string]string{"Content-Type": "text/plain"})
				decodeWireError(t, rec, http.StatusUnauthorized, "unauthenticated")
			})
		}
	}
	decodeWireError(t, connectPost(t, h, sessionsv1connect.SessionPublicServiceCreateSessionProcedure, `{not json`, nil),
		http.StatusBadRequest, "invalid_argument")
}

// Throttling counts attempts per account name whether or not the account exists, and answers both the same way.
func TestLoginThrottlePerAccount(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	throttled := map[string]error{}
	for _, account := range []string{"alice", "nobody"} {
		for i := range 5 {
			if _, err := a.signIn(account, "not the password", ""); connect.CodeOf(err) != connect.CodeUnauthenticated {
				t.Fatalf("%s attempt %d = %v, want unauthenticated", account, i+1, err)
			}
		}
		_, err := a.signIn(account, "not the password", "")
		assertConnectError(t, err, connect.CodeResourceExhausted, "LOGIN_THROTTLED")
		assertRetryInfo(t, err, time.Minute)
		throttled[account] = err
	}
	assertSameError(t, throttled["nobody"], throttled["alice"])
	// While waiting even the right password is refused; afterwards it signs in and the count starts over.
	_, err := a.signIn("alice", goodPassword, "")
	assertConnectError(t, err, connect.CodeResourceExhausted, "LOGIN_THROTTLED")
	a.clock.Advance(time.Minute)
	a.mustSignIn("alice", goodPassword)
	for range 4 {
		if _, err := a.signIn("alice", "not the password", ""); err == nil {
			t.Fatal("wrong password signed in")
		}
	}
	a.mustSignIn("alice", goodPassword)
}

// Attempts sent at once cannot exceed the allowance: each is counted before its password is checked.
func TestLoginThrottleConcurrent(t *testing.T) {
	a := newAuthAPI(t)
	a.hasher.delay = 50 * time.Millisecond
	a.mustSignUp("alice")
	outcomes := make(chan string, 20)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_, err := a.signIn("alice", "not the password", "")
			outcomes <- outcome(err)
		})
	}
	wg.Wait()
	close(outcomes)
	got := map[string]int{}
	for o := range outcomes {
		got[o]++
	}
	if got[connect.CodeUnauthenticated.String()] != 5 || got[connect.CodeResourceExhausted.String()] != 15 {
		t.Fatalf("outcomes = %v, want 5 unauthenticated and 15 resource_exhausted", got)
	}
	// what throttling protects: only 5 guesses were checked against the password
	if n := a.hasher.verifies.Load(); n != 5 {
		t.Fatalf("passwords verified = %d, want 5", n)
	}
}

// limitedCall returns a caller of procedure (sign-up or sign-in) whose calls with goodPassword succeed while the limit
// allows: sign-up creates account, sign-in signs in as alice whatever account says.
func (a *authAPI) limitedCall(procedure string) func(account, password string, opts ...requestOpt) error {
	if procedure == usersv1connect.UserPublicServiceCreateUserProcedure {
		return func(account, password string, opts ...requestOpt) error {
			_, err := a.signUp(account, password, opts...)
			return err
		}
	}
	return func(_, password string, opts ...requestOpt) error {
		_, err := a.signIn("alice", password, "", opts...)
		return err
	}
}

// Sign-up and sign-in share one per-address allowance (AUTH_RATE_LIMIT_PER_MIN), taken at the Connect entry before the
// request is decoded. Intended change (the move to Connect): a malformed or invalid request uses a token too, and over
// the limit it is resource_exhausted like any other. Procedures outside the limit, and other addresses, are not
// affected. The procedures are listed here, not read from connectLimited, so dropping one from it fails this test.
func TestRateLimitAuthOperations(t *testing.T) {
	for _, procedure := range []string{
		usersv1connect.UserPublicServiceCreateUserProcedure, sessionsv1connect.SessionPublicServiceCreateSessionProcedure,
	} {
		t.Run(procedure, func(t *testing.T) {
			a := newAuthAPI(t, func(_ *auth.Config, d *Deps) { d.RateLimitPerMin = 5 })
			_, alice, err := a.svc.CreateUser(t.Context(), "alice", goodPassword)
			if err != nil {
				t.Fatal(err)
			}
			call := a.limitedCall(procedure)
			for i := range 3 {
				if err := call(fmt.Sprintf("user%d", i), goodPassword); err != nil {
					t.Fatalf("request %d: %v", i+1, err)
				}
			}
			decodeWireError(t, connectPost(t, a.h, procedure, `{not json`, nil), http.StatusBadRequest, "invalid_argument")
			assertConnectError(t, call("user3", ""), connect.CodeInvalidArgument, "")

			err = call("user4", goodPassword)
			assertConnectError(t, err, connect.CodeResourceExhausted, "")
			// one token every 12s, and the test clock has not moved since the first request
			assertRetryInfo(t, err, 12*time.Second)
			decodeWireError(t, connectPost(t, a.h, procedure, `{not json`, nil), http.StatusTooManyRequests, "resource_exhausted")
			assertRateLimitScope(t, a, alice.Token, call)
		})
	}
}

// assertRateLimitScope checks, for an address over the limit, that sign-up and sign-in share its allowance, that
// another address has its own, and that other procedures are not limited.
func assertRateLimitScope(t *testing.T, a *authAPI, token string, call func(account, password string, opts ...requestOpt) error) {
	t.Helper()
	for _, procedure := range []string{
		usersv1connect.UserPublicServiceCreateUserProcedure, sessionsv1connect.SessionPublicServiceCreateSessionProcedure,
	} {
		decodeWireError(t, connectPost(t, a.h, procedure, `{}`, nil), http.StatusTooManyRequests, "resource_exhausted")
	}
	if err := call("user5", goodPassword, fromPeer("198.51.100.9:40000")); err != nil {
		t.Errorf("another address: %v", err)
	}
	if _, err := a.signOut(""); err != nil {
		t.Errorf("DeleteCurrentSession: %v", err)
	}
	assertMe(t, a, token, true)
}

// lockedBuffer is a log destination that requests running at once can share.
type lockedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Passwords and session tokens never reach the log, at any level, on success or failure paths: sign-up (including
// a failing breached-password check), wrong and right sign-in, a validation error and a malformed body carrying the
// password, an unknown token, and sign-out.
func TestCredentialsNeverLogged(t *testing.T) {
	// what must not leak, fresh each run: the phrase used as the password, and a cookie value the server does not know
	secretPart := rand.Text()
	canary := "canary " + secretPart + " phrase"
	strayCookie := rand.Text()
	var logs lockedBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	a := newAuthAPI(t, func(c *auth.Config, d *Deps) {
		c.Logger, d.Logger = logger, logger
		c.Breach = breachFunc(func(context.Context, string) (bool, error) { return false, errors.New("range API down") })
	})

	signUp, err := a.signUp("canary", canary)
	if err != nil {
		t.Fatalf("sign up: %v", err)
	}
	_, err = a.signIn("canary", canary+" wrong", "")
	assertConnectError(t, err, connect.CodeUnauthenticated, "INVALID_CREDENTIALS")
	token := a.mustSignIn("canary", canary)
	assertMe(t, a, token, true)
	_, err = a.signUp("x", canary)
	assertConnectError(t, err, connect.CodeInvalidArgument, "")
	decodeWireError(t, connectPost(t, a.h, sessionsv1connect.SessionPublicServiceCreateSessionProcedure,
		`{"account":"canary","password":"`+canary, nil), http.StatusBadRequest, "invalid_argument")
	assertMe(t, a, strayCookie, false)
	if _, err := a.signOut(token); err != nil {
		t.Fatal(err)
	}

	out := logs.String()
	if !strings.Contains(out, "breached-password check failed") ||
		!strings.Contains(out, `"path":"/api`+sessionsv1connect.SessionPublicServiceCreateSessionProcedure+`"`) {
		t.Fatalf("the log did not capture the requests: %s", out)
	}
	for _, leak := range []struct{ what, value string }{
		{"password", secretPart},
		{"sign-up session", a.sessionCookie(signUp.Header()).Value},
		{"sign-in session", token},
		{"unknown cookie", strayCookie},
	} {
		if strings.Contains(out, leak.value) {
			t.Errorf("the log contains the %s", leak.what)
		}
	}
}

// Sign-up, sign-in (both outcomes), sign-out and a refused admin method each log a security event with the user id
// when it is known (OWASP Logging Cheat Sheet, ASVS 5.0 16.3), so an incident can be traced to an account.
func TestSecurityEventsLogged(t *testing.T) {
	var logs lockedBuffer
	logger := slog.New(logx.WithContext(slog.NewJSONHandler(&logs, nil)))
	a := newAuthAPI(t, func(c *auth.Config, d *Deps) { c.Logger, d.Logger = logger, logger })
	alice, _ := a.mustSignUp("alice")
	for _, account := range []string{"alice", "nobody"} {
		_, err := a.signIn(account, "not the password", "")
		assertConnectError(t, err, connect.CodeUnauthenticated, "INVALID_CREDENTIALS")
	}
	token := a.mustSignIn("alice", goodPassword)
	_, err := a.admin(token).ListUsers(t.Context(), connect.NewRequest(&adminusersv1.ListUsersRequest{}))
	assertConnectError(t, err, connect.CodePermissionDenied, "INSUFFICIENT_ROLE")
	if _, err := a.signOut(token); err != nil {
		t.Fatal(err)
	}

	type event struct{ name, userID, reason string }
	var got []event
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		if name, ok := m["event"].(string); ok {
			if m["request_id"] == nil {
				t.Errorf("event %s has no request_id", name)
			}
			userID, _ := m["user_id"].(string)
			reason, _ := m["reason"].(string)
			got = append(got, event{name, userID, reason})
		}
	}
	id := alice.GetId()
	want := []event{
		{"auth.sign_up", id, ""},
		{"auth.sign_in_failed", id, "invalid_credentials"},
		{"auth.sign_in_failed", "", "unknown_account"},
		{"auth.sign_in", id, ""},
		{"authz.denied", id, ""},
		{"auth.sign_out", id, ""},
	}
	if !slices.Equal(got, want) {
		t.Errorf("events = %+v\nwant %+v", got, want)
	}
}
