package httpapi

import (
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	adminusersv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/admin/users/v1"
	sessionsv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/sessions/v1"
	usersv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/users/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/modules/auth"
)

// The user and session resources: reading users (UserAppService for oneself, UserAdminService for anyone), and
// listing, reading and revoking one's sessions (SessionAppService).

// pages follows next_page_token from the first page and returns every page's entries.
func pages[E any](t *testing.T, list func(pageToken string) ([]E, string, error)) [][]E {
	t.Helper()
	var all [][]E
	token := ""
	for range 100 {
		entries, next, err := list(token)
		if err != nil {
			t.Fatalf("page %d: %v", len(all)+1, err)
		}
		all = append(all, entries)
		if next == "" {
			return all
		}
		token = next
	}
	t.Fatal("pagination did not end")
	return nil
}

// listUsers returns every page of ListUsers, size users a page.
func (a *authAPI) listUsers(token string, size int32) [][]*usersv1.User {
	a.t.Helper()
	return pages(a.t, func(pageToken string) ([]*usersv1.User, string, error) {
		resp, err := a.admin(token).ListUsers(a.t.Context(), connect.NewRequest(&adminusersv1.ListUsersRequest{
			PageSize: size, PageToken: pageToken,
		}))
		if err != nil {
			return nil, "", err
		}
		return resp.Msg.GetUsers(), resp.Msg.GetNextPageToken(), nil
	})
}

// listSessions returns every page of ListSessions, size sessions a page.
func (a *authAPI) listSessions(token string, size int32) [][]*sessionsv1.Session {
	a.t.Helper()
	return pages(a.t, func(pageToken string) ([]*sessionsv1.Session, string, error) {
		resp, err := a.sessions(token).ListSessions(a.t.Context(), connect.NewRequest(&sessionsv1.ListSessionsRequest{
			PageSize: size, PageToken: pageToken,
		}))
		if err != nil {
			return nil, "", err
		}
		return resp.Msg.GetSessions(), resp.Msg.GetNextPageToken(), nil
	})
}

// A user reads her own account through UserAppService.GetUser, an admin any account through UserAdminService.GetUser.
// Intended change (the move to Connect): UserAppService.GetUser is self-only, so an admin asking it for another user
// gets not_found like anyone else; the REST getUser let an admin read anyone.
func TestGetUser(t *testing.T) {
	a := newAuthAPI(t)
	alice, aliceToken := a.mustSignUp("alice")
	admin := a.mustAdmin()
	if got, err := a.getUser(aliceToken, alice.GetId()); err != nil || !proto.Equal(got, alice) {
		t.Errorf("GetUser alice as herself = %v, %v; want %v", got, err, alice)
	}
	if got, err := a.adminGetUser(admin, alice.GetId()); err != nil || !proto.Equal(got, alice) {
		t.Errorf("UserAdminService.GetUser alice = %v, %v; want %v", got, err, alice)
	}
	_, err := a.getUser(admin, alice.GetId())
	assertConnectError(t, err, connect.CodeNotFound, "")
	_, err = a.adminGetUser(admin, uuid.Must(uuid.NewV7()).String())
	assertConnectError(t, err, connect.CodeNotFound, "")
	_, err = a.getUser(aliceToken, "42")
	assertFieldViolation(t, err, "id", "INVALID")
	_, err = a.adminGetUser(admin, "42")
	assertFieldViolation(t, err, "id", "INVALID")
}

// Another user's account is reported exactly like one that does not exist
// (docs/adr/2026-10-18-api-and-auth-baseline.md D7).
func TestGetOtherUserNotFound(t *testing.T) {
	a := newAuthAPI(t)
	alice, _ := a.mustSignUp("alice")
	_, bob := a.mustSignUp("bob")
	_, other := a.getUser(bob, alice.GetId())
	_, missing := a.getUser(bob, uuid.Must(uuid.NewV7()).String())
	assertConnectError(t, other, connect.CodeNotFound, "")
	assertSameError(t, other, missing)
	_, err := a.getUser("", alice.GetId())
	assertConnectError(t, err, connect.CodeUnauthenticated, "SESSION_INVALID")
}

func TestListUsers(t *testing.T) {
	a := newAuthAPI(t)
	admin := a.mustAdmin()
	want := map[string]bool{a.currentSession(admin).GetUser().GetId(): false}
	for _, account := range []string{"alice", "bob", "carol"} {
		u, _ := a.mustSignUp(account)
		want[u.GetId()] = account == "bob"
		if account == "bob" {
			if _, err := a.disable(admin, u.GetId(), true); err != nil {
				t.Fatalf("disable bob: %v", err)
			}
		}
	}
	got := a.listUsers(admin, 3)
	if len(got) != 2 || len(got[0]) != 3 || len(got[1]) != 1 {
		t.Fatalf("pages of 3 = %v, want 3 users then 1", got)
	}
	var ids []uuid.UUID
	for _, u := range slices.Concat(got...) {
		disabled, ok := want[u.GetId()]
		if !ok || u.GetDisabled() != disabled {
			t.Errorf("listed %v; want each user once, disabled ones included", u)
		}
		delete(want, u.GetId())
		ids = append(ids, uuid.MustParse(u.GetId()))
	}
	if len(want) > 0 {
		t.Errorf("not listed: %v", want)
	}
	if !slices.IsSortedFunc(ids, func(x, y uuid.UUID) int { return -compareUUID(x, y) }) {
		t.Errorf("ids = %v, want newest (largest UUIDv7) first", ids)
	}
}

func compareUUID(x, y uuid.UUID) int { return slices.Compare(x[:], y[:]) }

// page_size: 0 is the default 20, more than 100 is read as 100 (AIP-158), negative is invalid_argument; a page_token
// this API did not return is invalid_argument. With TestListSessionsPageParameters it replaces
// TestDefaultPageSizeMatchesContract: the page sizes are in the proto comments, not a contract default.
func TestListUsersPageParameters(t *testing.T) {
	a := newAuthAPI(t)
	admin := a.mustAdmin()
	if _, err := a.pool.Exec(t.Context(),
		"INSERT INTO users (account, password_hash) SELECT 'user' || g, 'x' FROM generate_series(1, 101) g"); err != nil {
		t.Fatal(err)
	}
	list := func(req *adminusersv1.ListUsersRequest) (*adminusersv1.ListUsersResponse, error) {
		resp, err := a.admin(admin).ListUsers(t.Context(), connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return resp.Msg, nil
	}
	for size, want := range map[int32]int{0: 20, 3: 3, 100: 100, 101: 100, 1000: 100} {
		page, err := list(&adminusersv1.ListUsersRequest{PageSize: size})
		if err != nil || len(page.GetUsers()) != want || page.GetNextPageToken() == "" {
			t.Errorf("page_size %d: %d users, next_page_token %q, %v; want %d and a token", size, len(page.GetUsers()),
				page.GetNextPageToken(), err, want)
		}
	}
	_, err := list(&adminusersv1.ListUsersRequest{PageSize: -1})
	assertFieldViolation(t, err, "page_size", "OUT_OF_RANGE")
	_, err = list(&adminusersv1.ListUsersRequest{PageToken: "not-a-token"})
	assertFieldViolation(t, err, "page_token", "INVALID")
}

func TestNonAdminCannotListUsers(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("mallory")
	_, err := a.admin(token).ListUsers(t.Context(), connect.NewRequest(&adminusersv1.ListUsersRequest{}))
	assertConnectError(t, err, connect.CodePermissionDenied, "INSUFFICIENT_ROLE")
	_, err = a.admin("").ListUsers(t.Context(), connect.NewRequest(&adminusersv1.ListUsersRequest{}))
	assertConnectError(t, err, connect.CodeUnauthenticated, "SESSION_INVALID")
}

// Signing in returns the session it created, which reads back with the session the response set (what followLocation
// checked for the 201's Location; the descriptor check requires CreateSession's GetSession).
func TestCreateSessionReturnsSession(t *testing.T) {
	a := newAuthAPI(t)
	created, _ := a.mustSignUp("alice")
	sess, token := a.mustSignInSession("alice")
	if !proto.Equal(sess.GetUser(), created) || !sess.GetCurrent() {
		t.Errorf("session = %v, want alice's current session", sess)
	}
	if id, err := uuid.Parse(sess.GetId()); err != nil || id.Version() != 7 {
		t.Errorf("id = %s, want a UUIDv7", sess.GetId())
	}
	now := a.clock.Now()
	if !sess.GetCreateTime().AsTime().Equal(now) || !sess.GetLastUseTime().AsTime().Equal(now) ||
		!sess.GetExpireTime().AsTime().Equal(now.Add(7*24*time.Hour)) {
		t.Errorf("times = %v, %v, %v; want now, now and now plus 7 days", sess.GetCreateTime(), sess.GetLastUseTime(),
			sess.GetExpireTime())
	}
	if got, err := a.getSession(token, sess.GetId()); err != nil || !proto.Equal(got, sess) {
		t.Errorf("GetSession with the new session = %v, %v; want %v", got, err, sess)
	}
}

func TestGetCurrentSession(t *testing.T) {
	a := newAuthAPI(t)
	created, token := a.mustSignUp("alice")
	current := a.currentSession(token)
	if !proto.Equal(current.GetUser(), created) || !current.GetCurrent() {
		t.Errorf("current session = %v, want alice's, marked current", current)
	}
	if got, err := a.getSession(token, current.GetId()); err != nil || !proto.Equal(got, current) {
		t.Errorf("GetSession %s = %v, %v; want the current session", current.GetId(), got, err)
	}
	_, err := a.sessions("").GetCurrentSession(t.Context(), connect.NewRequest(&sessionsv1.GetCurrentSessionRequest{}))
	assertConnectError(t, err, connect.CodeUnauthenticated, "SESSION_INVALID")
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
	ids := map[string]bool{}
	for _, s := range slices.Concat(got...) {
		if !proto.Equal(s.GetUser(), alice) {
			t.Errorf("listed %v, not alice's", s)
		}
		if s.GetCurrent() != (s.GetId() == second.GetId()) {
			t.Errorf("session %s current = %v; only %s is the one the request carries", s.GetId(), s.GetCurrent(), second.GetId())
		}
		ids[s.GetId()] = true
	}
	if len(ids) != 3 || !ids[a.currentSession(first).GetId()] || !ids[second.GetId()] {
		t.Errorf("listed %v, want alice's 3 sessions", ids)
	}
	if bobs := a.listSessions(bob, 20); len(bobs) != 1 || len(bobs[0]) != 1 {
		t.Errorf("bob's sessions = %v, want his one", bobs)
	}
}

// The page parameters of ListSessions, as TestListUsersPageParameters checks ListUsers'.
func TestListSessionsPageParameters(t *testing.T) {
	a := newAuthAPI(t)
	alice, token := a.mustSignUp("alice")
	if _, err := a.pool.Exec(t.Context(), `INSERT INTO sessions (token_hash, user_id, created_at, last_used_at, expires_at)
		SELECT sha256(('session ' || g)::bytea), $1::uuid, $2::timestamptz, $2::timestamptz, $2::timestamptz + interval '7 days'
		FROM generate_series(1, 101) g`, alice.GetId(), a.clock.Now()); err != nil {
		t.Fatal(err)
	}
	list := func(req *sessionsv1.ListSessionsRequest) (*sessionsv1.ListSessionsResponse, error) {
		resp, err := a.sessions(token).ListSessions(t.Context(), connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return resp.Msg, nil
	}
	for size, want := range map[int32]int{0: 20, 3: 3, 100: 100, 101: 100, 1000: 100} {
		page, err := list(&sessionsv1.ListSessionsRequest{PageSize: size})
		if err != nil || len(page.GetSessions()) != want || page.GetNextPageToken() == "" {
			t.Errorf("page_size %d: %d sessions, next_page_token %q, %v; want %d and a token", size,
				len(page.GetSessions()), page.GetNextPageToken(), err, want)
		}
	}
	_, err := list(&sessionsv1.ListSessionsRequest{PageSize: -1})
	assertFieldViolation(t, err, "page_size", "OUT_OF_RANGE")
	_, err = list(&sessionsv1.ListSessionsRequest{PageToken: "not-a-token"})
	assertFieldViolation(t, err, "page_token", "INVALID")
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
	if len(got) != 1 || len(got[0]) != 1 || got[0][0].GetId() != fresh.GetId() {
		t.Errorf("sessions = %v, want only %s", got, fresh.GetId())
	}
	_, err := a.getSession(token, stale.GetId())
	assertConnectError(t, err, connect.CodeNotFound, "")
	_, err = a.deleteSession(token, stale.GetId())
	assertConnectError(t, err, connect.CodeNotFound, "")

	// the absolute limit too: a session used every 12 hours ends 7 days after sign-in
	a = newAuthAPI(t)
	_, old = a.mustSignUp("alice")
	stale = a.currentSession(old)
	for range 13 {
		a.clock.Advance(12 * time.Hour)
		assertMe(t, a, old, true)
	}
	fresh, token = a.mustSignInSession("alice")
	a.clock.Advance(13 * time.Hour) // 7 days and 1 hour after the first sign-in, 13 hours after its last use
	got = a.listSessions(token, 20)
	if len(got) != 1 || len(got[0]) != 1 || got[0][0].GetId() != fresh.GetId() {
		t.Errorf("past the absolute limit: sessions = %v, want only %s", got, fresh.GetId())
	}
	_, err = a.getSession(token, stale.GetId())
	assertConnectError(t, err, connect.CodeNotFound, "")
}

// Revoking another of one's sessions leaves the cookie alone; revoking the one the request carries clears it.
func TestDeleteSession(t *testing.T) {
	a := newAuthAPI(t)
	_, first := a.mustSignUp("alice")
	second, secondToken := a.mustSignInSession("alice")
	resp, err := a.deleteSession(first, second.GetId())
	if err != nil {
		t.Fatalf("DeleteSession of the other session: %v", err)
	}
	if set := resp.Header().Values("Set-Cookie"); len(set) > 0 {
		t.Fatalf("DeleteSession of the other session: Set-Cookie %q, want none", set)
	}
	assertMe(t, a, secondToken, false)
	assertMe(t, a, first, true)
	resp, err = a.deleteSession(first, a.currentSession(first).GetId())
	if err != nil {
		t.Fatalf("DeleteSession of the current session: %v", err)
	}
	if c := a.sessionCookie(resp.Header()); c.MaxAge >= 0 {
		t.Errorf("Set-Cookie = %q, want the cookie cleared", resp.Header().Values("Set-Cookie"))
	}
	assertMe(t, a, first, false)
	_, live := a.mustSignUp("bob")
	_, err = a.deleteSession(live, "42")
	assertFieldViolation(t, err, "id", "INVALID")
}

// Another user's session cannot be read or revoked, and is reported like a missing one
// (docs/adr/2026-10-18-api-and-auth-baseline.md D7).
func TestDeleteOtherUsersSession(t *testing.T) {
	a := newAuthAPI(t)
	_, alice := a.mustSignUp("alice")
	target := a.currentSession(alice).GetId()
	_, bob := a.mustSignUp("bob")
	missing := uuid.Must(uuid.NewV7()).String()
	_, other := a.deleteSession(bob, target)
	_, none := a.deleteSession(bob, missing)
	assertConnectError(t, other, connect.CodeNotFound, "")
	assertSameError(t, other, none)
	_, other = a.getSession(bob, target)
	_, none = a.getSession(bob, missing)
	assertConnectError(t, other, connect.CodeNotFound, "")
	assertSameError(t, other, none)
	assertMe(t, a, alice, true)
}

// Deleting your own account needs a sign-in from the last 10 minutes; it removes the account, what it owns and its
// sessions, and clears the cookie.
func TestDeleteCurrentUser(t *testing.T) {
	a := newAuthAPI(t)
	alice, token := a.mustSignUp("alice")
	item := a.mustCreateItem(token, "mine")
	other := a.mustSignIn("alice", goodPassword)
	a.clock.Advance(11 * time.Minute)
	_, err := a.deleteCurrentUser(token)
	assertConnectError(t, err, connect.CodePermissionDenied, "REAUTHENTICATION_REQUIRED")

	fresh := a.mustSignIn("alice", goodPassword)
	resp, err := a.deleteCurrentUser(fresh)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if c := a.sessionCookie(resp.Header()); c.MaxAge >= 0 {
		t.Errorf("cookie not cleared: %v", c)
	}
	assertMe(t, a, other, false)
	_, err = a.signIn("alice", goodPassword, "")
	assertConnectError(t, err, connect.CodeUnauthenticated, "INVALID_CREDENTIALS")
	var items int
	if err := a.pool.QueryRow(t.Context(), `SELECT count(*) FROM items WHERE id = $1`, item.GetId()).Scan(&items); err != nil || items != 0 {
		t.Errorf("items left = %d (%v), want 0", items, err)
	}
	events := a.auditEvents()
	id := uuid.MustParse(alice.GetId())
	if e := events[len(events)-1]; e.action != auth.AuditUserDeleted || e.target != id || e.actor == nil || *e.actor != id {
		t.Errorf("audit row = %+v, want alice deleting herself", e)
	}
	// the name is free again
	a.mustSignUp("alice")
}

func TestDeleteCurrentUserLastAdmin(t *testing.T) {
	a := newAuthAPI(t)
	root := a.mustAdmin()
	_, err := a.deleteCurrentUser(root)
	assertConnectError(t, err, connect.CodeFailedPrecondition, "LAST_ADMIN")
}

func TestAdminDeletesUser(t *testing.T) {
	a := newAuthAPI(t)
	admin := a.mustAdmin()
	u, token := a.mustSignUp("alice")
	a.mustCreateItem(token, "mine")
	if err := a.deleteUser(admin, u.GetId()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	assertMe(t, a, token, false)
	_, err := a.adminGetUser(admin, u.GetId())
	assertConnectError(t, err, connect.CodeNotFound, "")
	assertConnectError(t, a.deleteUser(admin, u.GetId()), connect.CodeNotFound, "")
	assertFieldViolation(t, a.deleteUser(admin, "42"), "id", "INVALID")
	assertConnectError(t, a.deleteUser(admin, a.currentSession(admin).GetUser().GetId()), connect.CodeFailedPrecondition, "LAST_ADMIN")

	// a stale admin session cannot delete accounts either
	bob, _ := a.mustSignUp("bob")
	a.clock.Advance(11 * time.Minute)
	assertConnectError(t, a.deleteUser(admin, bob.GetId()), connect.CodePermissionDenied, "REAUTHENTICATION_REQUIRED")
}
