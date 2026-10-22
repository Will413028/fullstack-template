package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Will413028/fullstack-template/backend/internal/httpapi/gen"
	"github.com/Will413028/fullstack-template/backend/internal/modules/auth"
)

// The user and session resources (docs/adr/2026-10-18-api-and-auth-baseline.md D9): reading users, and listing,
// reading and revoking one's sessions.

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("body is not a %T: %v (%q)", v, err, rec.Body)
	}
	return v
}

func decodeSession(t *testing.T, rec *httptest.ResponseRecorder) gen.Session {
	t.Helper()
	return decodeJSON[gen.Session](t, rec)
}

func (a *authAPI) getUser(token string, id uuid.UUID) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.do(http.MethodGet, "/api/users/"+id.String(), token, "", "")
}

func (a *authAPI) sessionPath(id uuid.UUID) string { return "/api/sessions/" + id.String() }

// currentSession returns the session token authenticates.
func (a *authAPI) currentSession(token string) gen.Session {
	a.t.Helper()
	rec := a.do(http.MethodGet, "/api/sessions/current", token, "", "")
	if rec.Code != http.StatusOK {
		a.t.Fatalf("GET /api/sessions/current = %d %s", rec.Code, rec.Body)
	}
	return decodeSession(a.t, rec)
}

// mustSignInSession signs in and returns the new session and its token.
func (a *authAPI) mustSignInSession(account string) (gen.Session, string) {
	a.t.Helper()
	rec := a.signIn(account, goodPassword, "")
	if rec.Code != http.StatusCreated {
		a.t.Fatalf("sign in %s = %d %s", account, rec.Code, rec.Body)
	}
	return decodeSession(a.t, rec), sessionCookie(a.t, rec).Value
}

// pages follows next_cursor from the first page of path and returns every page's entries.
func pages[P any, E any](a *authAPI, token, path string, limit int, entries func(P) []E, next func(P) *string) [][]E {
	a.t.Helper()
	var all [][]E
	query := url.Values{"limit": {strconv.Itoa(limit)}}
	for {
		rec := a.do(http.MethodGet, path+"?"+query.Encode(), token, "", "")
		if rec.Code != http.StatusOK {
			a.t.Fatalf("GET %s?%s = %d %s", path, query.Encode(), rec.Code, rec.Body)
		}
		p := decodeJSON[P](a.t, rec)
		all = append(all, entries(p))
		c := next(p)
		if c == nil {
			return all
		}
		query.Set("cursor", *c)
	}
}

func cursorOf(p interface{ Get() (string, error) }) *string {
	c, err := p.Get()
	if err != nil {
		return nil
	}
	return &c
}

func TestGetUser(t *testing.T) {
	a := newAuthAPI(t)
	alice, aliceToken := a.mustSignUp("alice")
	admin := a.mustAdmin()
	for who, token := range map[string]string{"herself": aliceToken, "an admin": admin} {
		rec := a.getUser(token, alice.Id)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET alice as %s = %d %s", who, rec.Code, rec.Body)
		}
		if got := decodeUser(t, rec); got != alice {
			t.Errorf("GET alice as %s = %+v, want %+v", who, got, alice)
		}
	}
}

// Another user's account is reported exactly like one that does not exist
// (docs/adr/2026-10-18-api-and-auth-baseline.md D7).
func TestGetOtherUserNotFound(t *testing.T) {
	a := newAuthAPI(t)
	alice, _ := a.mustSignUp("alice")
	_, bob := a.mustSignUp("bob")
	other := assertProblem(t, a.getUser(bob, alice.Id), http.StatusNotFound, "/problems/not-found")
	missing := assertProblem(t, a.getUser(bob, uuid.Must(uuid.NewV7())), http.StatusNotFound, "/problems/not-found")
	other.RequestId, missing.RequestId = nil, nil
	if o, m := mustJSON(t, other), mustJSON(t, missing); o != m {
		t.Errorf("another user's account answered %s, a missing one %s; want the same", o, m)
	}
	assertProblem(t, a.getUser("", alice.Id), http.StatusUnauthorized, "/problems/unauthenticated")
}

func TestListUsers(t *testing.T) {
	a := newAuthAPI(t)
	admin := a.mustAdmin()
	want := map[uuid.UUID]bool{a.currentSession(admin).User.Id: false}
	for _, account := range []string{"alice", "bob", "carol"} {
		u, _ := a.mustSignUp(account)
		want[u.Id] = account == "bob"
		if account == "bob" {
			if rec := a.patchUser(admin, u.Id, `{"is_disabled":true}`); rec.Code != http.StatusOK {
				t.Fatalf("disable bob = %d %s", rec.Code, rec.Body)
			}
		}
	}
	got := pages(a, admin, "/api/users", 3,
		func(p gen.UserPage) []gen.User { return p.Users },
		func(p gen.UserPage) *string { return cursorOf(p.NextCursor) })
	if len(got) != 2 || len(got[0]) != 3 || len(got[1]) != 1 {
		t.Fatalf("pages of 3 = %v, want 3 users then 1", got)
	}
	var ids []uuid.UUID
	for _, u := range slices.Concat(got...) {
		disabled, ok := want[u.Id]
		if !ok || u.IsDisabled != disabled {
			t.Errorf("listed %+v; want each user once, disabled ones included", u)
		}
		delete(want, u.Id)
		ids = append(ids, u.Id)
	}
	if len(want) > 0 {
		t.Errorf("not listed: %v", want)
	}
	if !slices.IsSortedFunc(ids, func(x, y uuid.UUID) int { return -compareUUID(x, y) }) {
		t.Errorf("ids = %v, want newest (largest UUIDv7) first", ids)
	}
}

func compareUUID(x, y uuid.UUID) int { return slices.Compare(x[:], y[:]) }

func TestNonAdminCannotListUsers(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("mallory")
	p := assertProblem(t, a.do(http.MethodGet, "/api/users", token, "", ""), http.StatusForbidden, "/problems/forbidden")
	assertCode(t, p, "insufficient_role")
	assertProblem(t, a.do(http.MethodGet, "/api/users", "", "", ""), http.StatusUnauthorized, "/problems/unauthenticated")
}

// Signing in creates a session resource: the body is the session, Location its URL
// (docs/adr/2026-10-18-api-and-auth-baseline.md D9).
func TestCreateSessionReturnsSession(t *testing.T) {
	a := newAuthAPI(t)
	created, _ := a.mustSignUp("alice")
	rec := a.signIn("alice", goodPassword, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201 (body %q)", rec.Code, rec.Body)
	}
	sess := decodeSession(t, rec)
	if sess.User != created || !sess.Current {
		t.Errorf("session = %+v, want alice's current session", sess)
	}
	if sess.Id.Version() != 7 {
		t.Errorf("id = %s, want a UUIDv7", sess.Id)
	}
	if loc := rec.Header().Get("Location"); loc != a.sessionPath(sess.Id) {
		t.Errorf("Location = %q, want %s", loc, a.sessionPath(sess.Id))
	}
	now := a.clock.Now()
	if !sess.CreatedAt.Equal(now) || !sess.LastUsedAt.Equal(now) || !sess.ExpiresAt.Equal(now.Add(7*24*time.Hour)) {
		t.Errorf("times = %v, %v, %v; want now, now and now plus 7 days", sess.CreatedAt, sess.LastUsedAt, sess.ExpiresAt)
	}
}

func TestGetCurrentSession(t *testing.T) {
	a := newAuthAPI(t)
	created, token := a.mustSignUp("alice")
	current := a.currentSession(token)
	if current.User != created || !current.Current {
		t.Errorf("current session = %+v, want alice's, marked current", current)
	}
	rec := a.do(http.MethodGet, a.sessionPath(current.Id), token, "", "")
	if rec.Code != http.StatusOK || decodeSession(t, rec) != current {
		t.Errorf("GET %s = %d %s, want the current session", a.sessionPath(current.Id), rec.Code, rec.Body)
	}
	assertProblem(t, a.do(http.MethodGet, "/api/sessions/current", "", "", ""), http.StatusUnauthorized, "/problems/unauthenticated")
}

func (a *authAPI) listSessions(token string, limit int) [][]gen.Session {
	a.t.Helper()
	return pages(a, token, "/api/sessions", limit,
		func(p gen.SessionPage) []gen.Session { return p.Sessions },
		func(p gen.SessionPage) *string { return cursorOf(p.NextCursor) })
}

func TestListSessions(t *testing.T) {
	a := newAuthAPI(t)
	alice, first := a.mustSignUp("alice")
	a.mustSignInSession("alice")
	second, secondToken := a.mustSignInSession("alice")
	_, bob := a.mustSignUp("bob")
	got := a.listSessions(secondToken, 2)
	if len(got) != 2 || len(got[0]) != 2 || len(got[1]) != 1 {
		t.Fatalf("pages of 2 = %v, want 2 sessions then 1", got)
	}
	ids := map[uuid.UUID]bool{}
	for _, s := range slices.Concat(got...) {
		if s.User != alice {
			t.Errorf("listed %+v, not alice's", s)
		}
		if s.Current != (s.Id == second.Id) {
			t.Errorf("session %s current = %v; only %s is the one the request carries", s.Id, s.Current, second.Id)
		}
		ids[s.Id] = true
	}
	if len(ids) != 3 || !ids[a.currentSession(first).Id] || !ids[second.Id] {
		t.Errorf("listed %v, want alice's 3 sessions", ids)
	}
	if bobs := a.listSessions(bob, 20); len(bobs) != 1 || len(bobs[0]) != 1 {
		t.Errorf("bob's sessions = %v, want his one", bobs)
	}
}

// A session past its idle or absolute limit is gone from the API before Cleanup deletes its row.
func TestListSessionsExcludesExpired(t *testing.T) {
	a := newAuthAPI(t)
	_, old := a.mustSignUp("alice")
	stale := a.currentSession(old)
	a.clock.Advance(23 * time.Hour)
	fresh, token := a.mustSignInSession("alice")
	a.clock.Advance(2 * time.Hour) // the first session is now 25 hours unused, over the 1-day idle limit
	got := a.listSessions(token, 20)
	if len(got) != 1 || len(got[0]) != 1 || got[0][0].Id != fresh.Id {
		t.Errorf("sessions = %v, want only %s", got, fresh.Id)
	}
	assertProblem(t, a.do(http.MethodGet, a.sessionPath(stale.Id), token, "", ""), http.StatusNotFound, "/problems/not-found")
	assertProblem(t, a.do(http.MethodDelete, a.sessionPath(stale.Id), token, "", ""), http.StatusNotFound, "/problems/not-found")

	// the absolute limit too: a session used every 12 hours ends 7 days after sign-in
	a = newAuthAPI(t)
	_, old = a.mustSignUp("alice")
	stale = a.currentSession(old)
	for range 13 {
		a.clock.Advance(12 * time.Hour)
		assertMe(t, a, old, http.StatusOK)
	}
	fresh, token = a.mustSignInSession("alice")
	a.clock.Advance(13 * time.Hour) // 7 days and 1 hour after the first sign-in, 13 hours after its last use
	got = a.listSessions(token, 20)
	if len(got) != 1 || len(got[0]) != 1 || got[0][0].Id != fresh.Id {
		t.Errorf("past the absolute limit: sessions = %v, want only %s", got, fresh.Id)
	}
	assertProblem(t, a.do(http.MethodGet, a.sessionPath(stale.Id), token, "", ""), http.StatusNotFound, "/problems/not-found")
}

func TestDeleteSession(t *testing.T) {
	a := newAuthAPI(t)
	_, first := a.mustSignUp("alice")
	second, secondToken := a.mustSignInSession("alice")
	// revoking another of one's sessions leaves the cookie alone
	rec := a.do(http.MethodDelete, a.sessionPath(second.Id), first, "", "")
	if rec.Code != http.StatusNoContent || rec.Header().Get("Set-Cookie") != "" {
		t.Fatalf("DELETE the other session = %d, Set-Cookie %q; want 204 without", rec.Code, rec.Header().Get("Set-Cookie"))
	}
	assertMe(t, a, secondToken, http.StatusUnauthorized)
	assertMe(t, a, first, http.StatusOK)
	// revoking the current one also clears the cookie
	rec = a.do(http.MethodDelete, a.sessionPath(a.currentSession(first).Id), first, "", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE the current session = %d %s", rec.Code, rec.Body)
	}
	if c := sessionCookie(t, rec); c.MaxAge >= 0 {
		t.Errorf("Set-Cookie = %q, want the cookie cleared", rec.Header().Get("Set-Cookie"))
	}
	assertMe(t, a, first, http.StatusUnauthorized)
}

// Another user's session cannot be read or revoked, and is reported like a missing one
// (docs/adr/2026-10-18-api-and-auth-baseline.md D7).
func TestDeleteOtherUsersSession(t *testing.T) {
	a := newAuthAPI(t)
	_, alice := a.mustSignUp("alice")
	target := a.currentSession(alice)
	_, bob := a.mustSignUp("bob")
	assertProblem(t, a.do(http.MethodDelete, a.sessionPath(target.Id), bob, "", ""), http.StatusNotFound, "/problems/not-found")
	assertProblem(t, a.do(http.MethodGet, a.sessionPath(target.Id), bob, "", ""), http.StatusNotFound, "/problems/not-found")
	assertMe(t, a, alice, http.StatusOK)
}

func (a *authAPI) deleteMe(token string) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.do(http.MethodDelete, "/api/users/me", token, "", "")
}

// Deleting your own account needs a sign-in from the last 10 minutes; it removes the account, what it owns and its
// sessions, and clears the cookie.
func TestDeleteCurrentUser(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	item := a.mustCreateItem(token, "mine")
	other := a.mustSignIn("alice", goodPassword)
	a.clock.Advance(11 * time.Minute)
	p := assertProblem(t, a.deleteMe(token), http.StatusForbidden, "/problems/forbidden")
	assertCode(t, p, auth.CodeReauthenticationRequired)

	fresh := a.mustSignIn("alice", goodPassword)
	rec := a.deleteMe(fresh)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body)
	}
	if c := sessionCookie(t, rec); c.MaxAge >= 0 {
		t.Errorf("cookie not cleared: %v", c)
	}
	assertMe(t, a, other, http.StatusUnauthorized)
	assertProblem(t, a.signIn("alice", goodPassword, ""), http.StatusUnauthorized, "/problems/unauthenticated")
	var items int
	if err := a.pool.QueryRow(t.Context(), `SELECT count(*) FROM items WHERE id = $1`, item.Id).Scan(&items); err != nil || items != 0 {
		t.Errorf("items left = %d (%v), want 0", items, err)
	}
	// the name is free again
	a.mustSignUp("alice")
}

func TestDeleteCurrentUserLastAdmin(t *testing.T) {
	a := newAuthAPI(t)
	root := a.mustAdmin()
	p := assertProblem(t, a.deleteMe(root), http.StatusConflict, "/problems/conflict")
	assertCode(t, p, auth.CodeLastAdmin)
}

func TestAdminDeletesUser(t *testing.T) {
	a := newAuthAPI(t)
	admin := a.mustAdmin()
	u, token := a.mustSignUp("alice")
	a.mustCreateItem(token, "mine")
	path := "/api/users/" + u.Id.String()
	if rec := a.do(http.MethodDelete, path, admin, "", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body)
	}
	assertMe(t, a, token, http.StatusUnauthorized)
	assertProblem(t, a.getUser(admin, u.Id), http.StatusNotFound, "/problems/not-found")
	assertProblem(t, a.do(http.MethodDelete, path, admin, "", ""), http.StatusNotFound, "/problems/not-found")
	adminUser := a.currentSession(admin).User
	p := assertProblem(t, a.do(http.MethodDelete, "/api/users/"+adminUser.Id.String(), admin, "", ""),
		http.StatusConflict, "/problems/conflict")
	assertCode(t, p, auth.CodeLastAdmin)
}
