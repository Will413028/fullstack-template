package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Will413028/fullstack-template/backend/internal/httpapi/gen"
	"github.com/Will413028/fullstack-template/backend/internal/modules/auth"
	"github.com/Will413028/fullstack-template/backend/internal/modules/items"
	platformdb "github.com/Will413028/fullstack-template/backend/internal/platform/db"
	"github.com/Will413028/fullstack-template/backend/internal/platform/dbtest"
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

// authAPI is the real handler, with the auth and items services, over a migrated test database.
type authAPI struct {
	t      *testing.T
	h      http.Handler
	svc    *auth.Service
	pool   *pgxpool.Pool
	clock  *testClock
	hasher *countingHasher
}

type authOption func(*auth.Config, *Deps)

func newAuthAPI(t *testing.T, opts ...authOption) *authAPI {
	t.Helper()
	db := pg.NewMigratedDB(t)
	// opened the way serve opens it (TimeZone=UTC, timeouts)
	pool, err := platformdb.Open(t.Context(), platformdb.Config{
		URL: db.AppURL, Password: dbtest.AppRole, MaxConns: 10, StatementTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	a := &authAPI{t: t, pool: pool,
		clock:  &testClock{t: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)},
		hasher: &countingHasher{PasswordHasher: auth.NewArgon2Hasher(cheapArgon2, 0)},
	}
	cfg := auth.Config{SessionAbsolute: 7 * 24 * time.Hour, SessionIdle: 24 * time.Hour, Hasher: a.hasher, Now: a.clock.Now}
	deps := Deps{Ready: ok, Logger: discard, RateLimitPerMin: 1000, Cookie: Cookie{Name: cookieName, Secure: true},
		Items: items.New(pool)}
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

// do sends a same-origin request, with the session cookie when token is not empty.
func (a *authAPI) do(method, path, token, contentType, body string, headers ...string) *httptest.ResponseRecorder {
	a.t.Helper()
	req := httptest.NewRequestWithContext(a.t.Context(), method, "http://app.test"+path, strings.NewReader(body))
	if body == "" {
		req = httptest.NewRequestWithContext(a.t.Context(), method, "http://app.test"+path, http.NoBody)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: token}) //nolint:gosec // a request cookie has no attributes
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	return rec
}

func credentials(account, password string) string {
	b, _ := json.Marshal(map[string]string{"account": account, "password": password})
	return string(b)
}

func (a *authAPI) signUp(account, password string) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.do(http.MethodPost, "/api/users", "", "application/json", credentials(account, password))
}

func (a *authAPI) signIn(account, password, token string) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.do(http.MethodPost, "/api/sessions", token, "application/json", credentials(account, password))
}

func (a *authAPI) me(token string) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.do(http.MethodGet, "/api/users/me", token, "", "")
}

// sessionCookie returns the one session cookie rec sets.
func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != cookieName {
		t.Fatalf("Set-Cookie = %v, want exactly one %s", rec.Header().Values("Set-Cookie"), cookieName)
	}
	return cookies[0]
}

// mustSignUp creates an account and returns its user and session token.
func (a *authAPI) mustSignUp(account string) (gen.User, string) {
	a.t.Helper()
	rec := a.signUp(account, goodPassword)
	if rec.Code != http.StatusCreated {
		a.t.Fatalf("sign up %s = %d %s", account, rec.Code, rec.Body)
	}
	return decodeUser(a.t, rec), sessionCookie(a.t, rec).Value
}

func (a *authAPI) mustSignIn(account, password string) string {
	a.t.Helper()
	rec := a.signIn(account, password, "")
	if rec.Code != http.StatusCreated {
		a.t.Fatalf("sign in %s = %d %s", account, rec.Code, rec.Body)
	}
	return sessionCookie(a.t, rec).Value
}

// mustAdmin creates an admin the way seed-admin does and signs it in.
func (a *authAPI) mustAdmin() string {
	a.t.Helper()
	if _, err := a.svc.EnsureAdmin(a.t.Context(), "root", goodPassword); err != nil {
		a.t.Fatal(err)
	}
	return a.mustSignIn("root", goodPassword)
}

func decodeUser(t *testing.T, rec *httptest.ResponseRecorder) gen.User {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var u gen.User
	if err := json.Unmarshal(rec.Body.Bytes(), &u); err != nil {
		t.Fatalf("body is not a User: %v (%q)", err, rec.Body)
	}
	return u
}

func assertMe(t *testing.T, a *authAPI, token string, want int) {
	t.Helper()
	rec := a.me(token)
	if rec.Code != want {
		t.Fatalf("GET /api/users/me = %d, want %d (body %q)", rec.Code, want, rec.Body)
	}
	if want == http.StatusUnauthorized {
		assertProblem(t, rec, http.StatusUnauthorized, "/problems/unauthenticated")
	}
}

func assertCode(t *testing.T, p gen.ValidationProblem, code string) {
	t.Helper()
	if p.Code == nil || *p.Code != code {
		t.Errorf("problem code = %v, want %s", p.Code, code)
	}
}

func assertFieldError(t *testing.T, p gen.ValidationProblem, pointer, code string) {
	t.Helper()
	if len(p.Errors) != 1 || p.Errors[0].Pointer != pointer || p.Errors[0].Code != code {
		t.Errorf("errors = %+v, want one %s at %s", p.Errors, code, pointer)
	}
}

func TestOperationsRejectsUnknownName(t *testing.T) {
	if _, err := operations("CreateUser", "createUser"); err == nil {
		t.Fatal("the contract's operationId was accepted; strict middleware sees the generated method name")
	}
}

func TestCreateUserSetsSessionCookie(t *testing.T) {
	a := newAuthAPI(t)
	rec := a.signUp("Alice", goodPassword)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201 (body %q)", rec.Code, rec.Body)
	}
	u := decodeUser(t, rec)
	if u.Account != "alice" || u.Role != gen.UserRoleUser || u.IsDisabled {
		t.Errorf("user = %+v, want a lowercase, enabled user", u)
	}
	var raw struct {
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil || !strings.HasSuffix(raw.CreatedAt, "Z") {
		t.Errorf("created_at = %q, want RFC 3339 in UTC", raw.CreatedAt)
	}
	if loc := rec.Header().Get("Location"); loc != "/api/users/"+u.Id.String() {
		t.Errorf("Location = %q", loc)
	}
	c := sessionCookie(t, rec)
	if !c.HttpOnly || !c.Secure || c.Path != "/" || c.SameSite != http.SameSiteLaxMode || c.Domain != "" {
		t.Errorf("cookie %q lacks HttpOnly, Secure, Path=/ or SameSite=Lax, or has a Domain", rec.Header().Get("Set-Cookie"))
	}
	if c.MaxAge != 7*24*3600 {
		t.Errorf("Max-Age = %d, want the 7-day absolute lifetime", c.MaxAge)
	}
	if len(c.Value) != 43 {
		t.Errorf("token %q is not 32 bytes of base64url", c.Value)
	}
	assertMe(t, a, c.Value, http.StatusOK)
}

func TestCreateUserDuplicate(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	p := assertProblem(t, a.signUp("ALICE", goodPassword), http.StatusConflict, "/problems/conflict")
	assertCode(t, p, auth.CodeAccountTaken)
}

func TestCreateUserPasswordTooShort(t *testing.T) {
	a := newAuthAPI(t)
	p := assertProblem(t, a.signUp("alice", "fourteen chars"), http.StatusUnprocessableEntity, "/problems/validation")
	assertFieldError(t, p, "/password", "too_short")
}

// NIST SP 800-63B-4 forbids composition rules: a long all-lowercase password is fine.
func TestCreateUserNoCompositionRule(t *testing.T) {
	a := newAuthAPI(t)
	if rec := a.signUp("alice", "onlylowercaseletters"); rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201 (body %q)", rec.Code, rec.Body)
	}
}

func TestCreateUserCommonPassword(t *testing.T) {
	a := newAuthAPI(t)
	// on the embedded list (SecLists Pwdb), 15 characters
	p := assertProblem(t, a.signUp("alice", "demon1q2w3e4r5t"), http.StatusUnprocessableEntity, "/problems/validation")
	assertFieldError(t, p, "/password", auth.CodePasswordTooCommon)
}

func TestCreateUserBreachedPassword(t *testing.T) {
	var checked string
	breached := true
	var failing error
	a := newAuthAPI(t, func(c *auth.Config, _ *Deps) {
		c.Breach = breachFunc(func(_ context.Context, p string) (bool, error) { checked = p; return breached, failing })
	})
	// The check sees the normalized password (NFKC folds the full-width letters).
	p := assertProblem(t, a.signUp("alice", "ｂｒｅａｃｈｅｄ ｐａｓｓｐｈｒａｓｅ"), http.StatusUnprocessableEntity, "/problems/validation")
	assertFieldError(t, p, "/password", auth.CodePasswordTooCommon)
	if checked != "breached passphrase" {
		t.Errorf("breach check got %q, want the NFKC form", checked)
	}
	// An unreachable breach service does not block sign-ups.
	failing = errors.New("timeout")
	if rec := a.signUp("alice", goodPassword); rec.Code != http.StatusCreated {
		t.Fatalf("with the breach check failing: code = %d, want 201", rec.Code)
	}
}

func TestCreateSession(t *testing.T) {
	a := newAuthAPI(t)
	created, _ := a.mustSignUp("alice")
	rec := a.signIn("alice", goodPassword, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201 (body %q)", rec.Code, rec.Body)
	}
	if u := decodeUser(t, rec); u.Id != created.Id {
		t.Errorf("signed in as %v, want %v", u.Id, created.Id)
	}
	if loc := rec.Header().Get("Location"); loc != "/api/sessions/current" {
		t.Errorf("Location = %q", loc)
	}
	assertMe(t, a, sessionCookie(t, rec).Value, http.StatusOK)
}

func TestCreateSessionWrongPassword(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	p := assertProblem(t, a.signIn("alice", "not the password", ""), http.StatusUnauthorized, "/problems/unauthenticated")
	assertCode(t, p, auth.CodeInvalidCredentials)
}

// An unknown account costs one password verification, like a wrong password, and gets the same response.
func TestCreateSessionUnknownAccountRunsHash(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	wrong := assertProblem(t, a.signIn("alice", "not the password", ""), http.StatusUnauthorized, "/problems/unauthenticated")
	a.hasher.verifies.Store(0)
	unknown := assertProblem(t, a.signIn("bob", "not the password", ""), http.StatusUnauthorized, "/problems/unauthenticated")
	if n := a.hasher.verifies.Load(); n != 1 {
		t.Errorf("verifications for an unknown account = %d, want 1", n)
	}
	wrong.RequestId, unknown.RequestId = nil, nil
	if w, u := mustJSON(t, wrong), mustJSON(t, unknown); w != u {
		t.Errorf("unknown account answered %s, wrong password %s; want the same", u, w)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Signing in always issues a new token (no session fixation) and ends the session the request carried.
func TestCreateSessionIssuesNewToken(t *testing.T) {
	a := newAuthAPI(t)
	_, old := a.mustSignUp("alice")
	rec := a.signIn("alice", goodPassword, old)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d (body %q)", rec.Code, rec.Body)
	}
	fresh := sessionCookie(t, rec).Value
	if fresh == old {
		t.Fatal("sign-in kept the token the request carried")
	}
	assertMe(t, a, fresh, http.StatusOK)
	assertMe(t, a, old, http.StatusUnauthorized)
}

func TestCreateSessionMixedCaseAccount(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	a.mustSignIn("ALICE", goodPassword)
}

// The same password typed with full-width or half-width characters is the same password (NFKC, plan P7).
func TestPasswordNormalized(t *testing.T) {
	a := newAuthAPI(t)
	if rec := a.signUp("alice", "ｃｏｒｒｅｃｔ－ｈｏｒｓｅ－ｂａｔｔｅｒｙ"); rec.Code != http.StatusCreated {
		t.Fatalf("sign up = %d (body %q)", rec.Code, rec.Body)
	}
	a.mustSignIn("alice", "correct-horse-battery")
	if rec := a.signUp("bob", "correct-horse-battery-staple"); rec.Code != http.StatusCreated {
		t.Fatalf("sign up = %d (body %q)", rec.Code, rec.Body)
	}
	a.mustSignIn("bob", "ｃｏｒｒｅｃｔ－ｈｏｒｓｅ－ｂａｔｔｅｒｙ－ｓｔａｐｌｅ")
}

func TestCreateSessionDisabledAccount(t *testing.T) {
	a := newAuthAPI(t)
	u, _ := a.mustSignUp("alice")
	if _, err := a.svc.SetDisabled(t.Context(), u.Id, true); err != nil {
		t.Fatal(err)
	}
	// the same answer as a wrong password: a disabled account is not told apart
	p := assertProblem(t, a.signIn("alice", goodPassword, ""), http.StatusUnauthorized, "/problems/unauthenticated")
	assertCode(t, p, auth.CodeInvalidCredentials)
}

func TestMe(t *testing.T) {
	a := newAuthAPI(t)
	created, token := a.mustSignUp("alice")
	rec := a.me(token)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d (body %q)", rec.Code, rec.Body)
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if id, _ := raw["id"].(string); id != created.Id.String() || uuid.MustParse(id).Version() != 7 {
		t.Errorf("id = %v, want the UUIDv7 string %s", raw["id"], created.Id)
	}
	if raw["role"] != "user" || raw["account"] != "alice" {
		t.Errorf("body = %v, want account alice with role user", raw)
	}
}

func TestMeWithoutCookie(t *testing.T) {
	assertMe(t, newAuthAPI(t), "", http.StatusUnauthorized)
}

func TestMeWithUnknownToken(t *testing.T) {
	assertMe(t, newAuthAPI(t), "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", http.StatusUnauthorized)
}

// Only the cookie authenticates; a token in Authorization is ignored.
// The cookie is HttpOnly, so only the server can remove a dead one; the frontend takes a present cookie for a
// session.
func TestInvalidSessionCookieCleared(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	a.do(http.MethodDelete, "/api/sessions/current", token, "", "")
	rec := a.me(token)
	assertProblem(t, rec, http.StatusUnauthorized, "/problems/unauthenticated")
	if c := sessionCookie(t, rec); c.MaxAge >= 0 {
		t.Errorf("Set-Cookie = %q, want the cookie cleared", rec.Header().Get("Set-Cookie"))
	}
	// signing in with the dead cookie gets only the new one
	a.mustSignUp("bob")
	rec = a.signIn("bob", goodPassword, token)
	if c := sessionCookie(t, rec); c.MaxAge <= 0 || c.Value == "" {
		t.Errorf("Set-Cookie = %v, want only the new session", rec.Header().Values("Set-Cookie"))
	}
	// a valid session is left alone
	_, live := a.mustSignUp("carol")
	if rec := a.me(live); rec.Header().Get("Set-Cookie") != "" {
		t.Errorf("valid session got Set-Cookie %q", rec.Header().Get("Set-Cookie"))
	}
}

func TestMeIgnoresBearer(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	rec := a.do(http.MethodGet, "/api/users/me", "", "", "", "Authorization", "Bearer "+token)
	assertProblem(t, rec, http.StatusUnauthorized, "/problems/unauthenticated")
}

func TestMeDisabledAccount(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	// disabled without going through SetDisabled, so the session row still exists
	if _, err := a.pool.Exec(t.Context(), `UPDATE users SET is_disabled = true`); err != nil {
		t.Fatal(err)
	}
	assertMe(t, a, token, http.StatusUnauthorized)
}

func TestMeAfterUserDeleted(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	if _, err := a.pool.Exec(t.Context(), `DELETE FROM users`); err != nil {
		t.Fatal(err)
	}
	assertMe(t, a, token, http.StatusUnauthorized)
}

func TestSessionIdleExpiry(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	a.clock.Advance(24*time.Hour - time.Second)
	assertMe(t, a, token, http.StatusOK) // using it resets the idle limit
	a.clock.Advance(24*time.Hour - time.Second)
	assertMe(t, a, token, http.StatusOK)
	a.clock.Advance(24 * time.Hour)
	assertMe(t, a, token, http.StatusUnauthorized)
}

func TestSessionAbsoluteExpiry(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	for range 13 {
		a.clock.Advance(12 * time.Hour)
		assertMe(t, a, token, http.StatusOK)
	}
	a.clock.Advance(12 * time.Hour) // 7 days after sign-up, used every 12 hours
	assertMe(t, a, token, http.StatusUnauthorized)
}

func TestDeleteCurrentSessionClearsCookie(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	for _, tok := range []string{token, ""} { // without a session it still succeeds
		rec := a.do(http.MethodDelete, "/api/sessions/current", tok, "", "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("code = %d, want 204 (body %q)", rec.Code, rec.Body)
		}
		if c := sessionCookie(t, rec); c.MaxAge >= 0 || !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
			t.Errorf("Set-Cookie = %q, want Max-Age=0", rec.Header().Get("Set-Cookie"))
		}
	}
}

func TestDeleteCurrentSessionRevokes(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	other := a.mustSignIn("alice", goodPassword)
	a.do(http.MethodDelete, "/api/sessions/current", token, "", "")
	assertMe(t, a, token, http.StatusUnauthorized)
	assertMe(t, a, other, http.StatusOK)
}

func TestDeleteAllSessions(t *testing.T) {
	a := newAuthAPI(t)
	_, first := a.mustSignUp("alice")
	second := a.mustSignIn("alice", goodPassword)
	_, bob := a.mustSignUp("bob")
	rec := a.do(http.MethodDelete, "/api/sessions", second, "", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204 (body %q)", rec.Code, rec.Body)
	}
	if c := sessionCookie(t, rec); c.MaxAge >= 0 {
		t.Errorf("cookie not cleared: %q", rec.Header().Get("Set-Cookie"))
	}
	assertMe(t, a, first, http.StatusUnauthorized)
	assertMe(t, a, second, http.StatusUnauthorized)
	assertMe(t, a, bob, http.StatusOK)
	assertProblem(t, a.do(http.MethodDelete, "/api/sessions", "", "", ""), http.StatusUnauthorized, "/problems/unauthenticated")
}

func (a *authAPI) patchUser(token string, id uuid.UUID, body string) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.do(http.MethodPatch, "/api/users/"+id.String(), token, "application/merge-patch+json", body)
}

// Disabling ends the user's sessions for good: enabling the account again does not bring them back.
func TestAdminDisablesUserRevokesSessions(t *testing.T) {
	a := newAuthAPI(t)
	admin := a.mustAdmin()
	u, token := a.mustSignUp("alice")
	rec := a.patchUser(admin, u.Id, `{"is_disabled":true}`)
	if rec.Code != http.StatusOK || !decodeUser(t, rec).IsDisabled {
		t.Fatalf("disable = %d %s", rec.Code, rec.Body)
	}
	assertMe(t, a, token, http.StatusUnauthorized)
	rec = a.patchUser(admin, u.Id, `{"is_disabled":false}`)
	if rec.Code != http.StatusOK || decodeUser(t, rec).IsDisabled {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body)
	}
	assertMe(t, a, token, http.StatusUnauthorized)
	a.mustSignIn("alice", goodPassword)
	// an empty merge patch changes nothing
	if rec := a.patchUser(admin, u.Id, `{}`); rec.Code != http.StatusOK || decodeUser(t, rec).IsDisabled {
		t.Fatalf("empty patch = %d %s", rec.Code, rec.Body)
	}
	assertProblem(t, a.patchUser(admin, uuid.Must(uuid.NewV7()), `{"is_disabled":true}`), http.StatusNotFound, "/problems/not-found")
}

func TestNonAdminCannotPatchUser(t *testing.T) {
	a := newAuthAPI(t)
	victim, victimToken := a.mustSignUp("alice")
	_, token := a.mustSignUp("mallory")
	p := assertProblem(t, a.patchUser(token, victim.Id, `{"is_disabled":true}`), http.StatusForbidden, "/problems/forbidden")
	assertCode(t, p, "insufficient_role")
	assertMe(t, a, victimToken, http.StatusOK)
}

// Security is checked before the body: an anonymous caller gets 401, not a 422 describing the body it should send.
func TestUnauthenticatedBeforeValidation(t *testing.T) {
	a := newAuthAPI(t)
	rec := a.patchUser("", uuid.Must(uuid.NewV7()), `{"is_disabled":"yes"}`)
	assertProblem(t, rec, http.StatusUnauthorized, "/problems/unauthenticated")
}

// Throttling counts attempts per account name whether or not the account exists, and answers both the same way.
func TestLoginThrottlePerAccount(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	throttled := map[string]gen.ValidationProblem{}
	for _, account := range []string{"alice", "nobody"} {
		for i := range 5 {
			if rec := a.signIn(account, "not the password", ""); rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s attempt %d = %d, want 401", account, i+1, rec.Code)
			}
		}
		rec := a.signIn(account, "not the password", "")
		p := assertProblem(t, rec, http.StatusTooManyRequests, "/problems/rate-limited")
		assertCode(t, p, auth.CodeLoginThrottled)
		if ra := rec.Header().Get("Retry-After"); ra != "60" {
			t.Errorf("%s Retry-After = %q, want 60", account, ra)
		}
		p.RequestId = nil
		throttled[account] = p
	}
	if e, n := mustJSON(t, throttled["alice"]), mustJSON(t, throttled["nobody"]); e != n {
		t.Errorf("existing account throttled with %s, unknown with %s; want the same", e, n)
	}
	// While waiting even the right password is refused; afterwards it signs in and the count starts over.
	assertProblem(t, a.signIn("alice", goodPassword, ""), http.StatusTooManyRequests, "/problems/rate-limited")
	a.clock.Advance(time.Minute)
	a.mustSignIn("alice", goodPassword)
	for range 4 {
		a.signIn("alice", "not the password", "")
	}
	a.mustSignIn("alice", goodPassword)
}

// Attempts sent at once cannot exceed the allowance: each is counted before its password is checked.
func TestLoginThrottleConcurrent(t *testing.T) {
	a := newAuthAPI(t)
	a.hasher.delay = 50 * time.Millisecond
	a.mustSignUp("alice")
	var unauthorized, limited atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			switch code := a.signIn("alice", "not the password", "").Code; code {
			case http.StatusUnauthorized:
				unauthorized.Add(1)
			case http.StatusTooManyRequests:
				limited.Add(1)
			default:
				t.Errorf("code = %d", code)
			}
		})
	}
	wg.Wait()
	if unauthorized.Load() != 5 || limited.Load() != 15 {
		t.Fatalf("%d checked and %d throttled, want 5 and 15", unauthorized.Load(), limited.Load())
	}
	// what throttling protects: only 5 guesses were checked against the password
	if n := a.hasher.verifies.Load(); n != 5 {
		t.Fatalf("passwords verified = %d, want 5", n)
	}
}

// Sign-up and sign-in share one per-address allowance (AUTH_RATE_LIMIT_PER_MIN).
func TestRateLimitAuthOperations(t *testing.T) {
	for _, op := range []string{"createUser", "createSession"} {
		t.Run(op, func(t *testing.T) {
			a := newAuthAPI(t, func(_ *auth.Config, d *Deps) { d.RateLimitPerMin = 2 })
			if _, _, err := a.svc.CreateUser(t.Context(), "alice", goodPassword); err != nil {
				t.Fatal(err)
			}
			send := func(i int) *httptest.ResponseRecorder {
				if op == "createUser" {
					return a.signUp(fmt.Sprintf("user%d", i), goodPassword)
				}
				return a.signIn("alice", goodPassword, "")
			}
			for i := range 2 {
				if code := send(i).Code; code != http.StatusCreated {
					t.Fatalf("request %d = %d, want 201", i+1, code)
				}
			}
			rec := send(2)
			assertProblem(t, rec, http.StatusTooManyRequests, "/problems/rate-limited")
			// one token every 30s, less the time the first requests took (the limiter uses the real clock)
			if ra, err := strconv.Atoi(rec.Header().Get("Retry-After")); err != nil || ra < 1 || ra > 30 {
				t.Errorf("Retry-After = %q, want 1 to 30 seconds", rec.Header().Get("Retry-After"))
			}
		})
	}
}
