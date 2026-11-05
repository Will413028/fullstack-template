package httpapi

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	adminusersv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/admin/users/v1"
	sessionsv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/sessions/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/modules/auth"
)

const deviceCookieName = "__Host-device"

// signInFrom signs in from the client address ip, with the device cookie when device is not empty.
func (a *authAPI) signInFrom(ip, account, password, device string) (*connect.Response[sessionsv1.CreateSessionResponse], error) {
	a.t.Helper()
	opts := []requestOpt{fromPeer(ip + ":40000")}
	if device != "" {
		opts = append(opts, withCookie(deviceCookieName, device))
	}
	return a.signIn(account, password, "", opts...)
}

// deviceCookie returns the device cookie h sets, nil without one.
func deviceCookie(h http.Header) *http.Cookie {
	for _, c := range responseCookies(h) {
		if c.Name == deviceCookieName {
			return c
		}
	}
	return nil
}

// mustDeviceCookie is the device cookie h sets; a test that needs it fails without it (rather than dereferencing
// nil, which would abort the package and hide every other test's result).
func mustDeviceCookie(t *testing.T, h http.Header) *http.Cookie {
	t.Helper()
	c := deviceCookie(h)
	if c == nil {
		t.Fatalf("no device cookie in %q", h.Values("Set-Cookie"))
	}
	return c
}

// lockOut fails n sign-ins for account, each from its own address, so only the account-wide count can stop them.
func (a *authAPI) lockOut(account string, n int) {
	a.t.Helper()
	for i := range n {
		_, err := a.signInFrom(fmt.Sprintf("198.51.%d.%d", i/200, i%200+1), account, "not the password", "")
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			a.t.Fatalf("failure %d = %v, want unauthenticated", i+1, err)
		}
	}
}

// A browser's first sign-in to an account gets two cookies on the one response: the session, and the device cookie
// that the account trusts from then on (two Set-Cookie headers, Header().Add). A sign-in that carries the device
// cookie gets only the session.
func TestFirstSignInSetsTwoCookies(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	resp, err := a.signInFrom("203.0.113.1", "alice", goodPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	set := resp.Header().Values("Set-Cookie")
	device := deviceCookie(resp.Header())
	if len(set) != 2 || device == nil || a.sessionCookie(resp.Header()).MaxAge <= 0 {
		t.Fatalf("first sign-in: Set-Cookie = %q, want the session and the device cookie", set)
	}
	resp, err = a.signInFrom("203.0.113.1", "alice", goodPassword, device.Value)
	if err != nil {
		t.Fatal(err)
	}
	if set := resp.Header().Values("Set-Cookie"); len(set) != 1 || deviceCookie(resp.Header()) != nil {
		t.Errorf("sign-in from the trusted device: Set-Cookie = %q, want only the session", set)
	}
}

// One client's failures make that client wait; another client signs in to the same account at once.
func TestSignInThrottlePerClient(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	for i := range 5 {
		if _, err := a.signInFrom("198.51.100.1", "alice", "not the password", ""); connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Fatalf("attempt %d = %v, want unauthenticated", i+1, err)
		}
	}
	_, err := a.signInFrom("198.51.100.1", "alice", goodPassword, "")
	assertConnectError(t, err, connect.CodeResourceExhausted, "LOGIN_THROTTLED")
	if _, err := a.signInFrom("198.51.100.2", "alice", goodPassword, ""); err != nil {
		t.Errorf("another client: %v", err)
	}
}

// After 100 failures from clients it does not know, the account refuses them all, but a browser that signed in
// before (device cookie) still gets in, and its sign-in clears the lock (NIST SP 800-63B 3.2.2).
func TestUntrustedClientsLockedTrustedDeviceNot(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	resp, err := a.signInFrom("203.0.113.1", "alice", goodPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	device := deviceCookie(resp.Header())
	if device == nil {
		t.Fatalf("first sign-in: no device cookie in %q", resp.Header().Values("Set-Cookie"))
	}
	if !device.HttpOnly || !device.Secure || device.SameSite != http.SameSiteStrictMode || device.Path != "/" ||
		device.MaxAge != int((180*24*time.Hour).Seconds()) {
		t.Errorf("device cookie = %+v", device)
	}
	a.lockOut("alice", 99)
	if _, err := a.signInFrom("192.0.2.10", "alice", goodPassword, ""); err != nil {
		t.Fatalf("after 99 failures: %v, want a session (and the count starts over)", err)
	}
	a.lockOut("alice", 100)
	_, err = a.signInFrom("192.0.2.11", "alice", goodPassword, "")
	assertConnectError(t, err, connect.CodeResourceExhausted, "LOGIN_THROTTLED")
	if _, err := a.signInFrom("192.0.2.12", "alice", goodPassword, device.Value); err != nil {
		t.Fatalf("trusted device: %v", err)
	}
	if _, err := a.signInFrom("192.0.2.13", "alice", goodPassword, ""); err != nil {
		t.Errorf("after the owner signed in: %v", err)
	}
}

// A device cookie of another account does not exempt a client.
func TestDeviceCookieIsPerAccount(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	a.mustSignUp("mallory")
	resp, err := a.signInFrom("203.0.113.1", "mallory", goodPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	device := mustDeviceCookie(t, resp.Header())
	a.lockOut("alice", 100)
	_, err = a.signInFrom("203.0.113.1", "alice", goodPassword, device.Value)
	assertConnectError(t, err, connect.CodeResourceExhausted, "LOGIN_THROTTLED")
}

// Signing out everywhere forgets the account's devices too.
func TestSignOutEverywhereForgetsDevices(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	resp, err := a.signInFrom("203.0.113.1", "alice", goodPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	device, token := mustDeviceCookie(t, resp.Header()), a.sessionCookie(resp.Header()).Value
	if _, err := a.sessions(token).DeleteAllSessions(t.Context(), connect.NewRequest(&sessionsv1.DeleteAllSessionsRequest{})); err != nil {
		t.Fatalf("sign out everywhere: %v", err)
	}
	a.lockOut("alice", 100)
	_, err = a.signInFrom("203.0.113.1", "alice", goodPassword, device.Value)
	assertConnectError(t, err, connect.CodeResourceExhausted, "LOGIN_THROTTLED")
}

func (a *authAPI) signInLock(token, userID string) (*adminusersv1.SignInLock, error) {
	a.t.Helper()
	resp, err := a.admin(token).GetSignInLock(a.t.Context(), connect.NewRequest(&adminusersv1.GetSignInLockRequest{UserId: userID}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetSignInLock(), nil
}

func (a *authAPI) resetSignInLock(token, userID string) error {
	a.t.Helper()
	_, err := a.admin(token).ResetSignInLock(a.t.Context(), connect.NewRequest(&adminusersv1.ResetSignInLockRequest{UserId: userID}))
	return err
}

// An admin sees and lifts the lock; the unlock is audited.
func TestAdminUnlocksSignIn(t *testing.T) {
	a := newAuthAPI(t)
	admin := a.mustAdmin()
	alice, _ := a.mustSignUp("alice")
	if lock, err := a.signInLock(admin, alice.GetId()); err != nil || lock.GetLocked() || lock.GetFailures() != 0 {
		t.Errorf("before = %v, %v", lock, err)
	}
	a.lockOut("alice", 100)
	if lock, err := a.signInLock(admin, alice.GetId()); err != nil || !lock.GetLocked() || lock.GetFailures() != 100 {
		t.Errorf("locked = %v, %v; want locked after 100 failures", lock, err)
	}
	if err := a.resetSignInLock(admin, alice.GetId()); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if _, err := a.signInFrom("192.0.2.20", "alice", goodPassword, ""); err != nil {
		t.Errorf("after unlock: %v", err)
	}
	events := a.auditEvents()
	if e := events[len(events)-1]; e.action != auth.AuditUserUnlocked || e.target != uuid.MustParse(alice.GetId()) {
		t.Errorf("audit row = %+v, want user.unlocked for alice", e)
	}
	missing := uuid.Must(uuid.NewV7()).String()
	_, err := a.signInLock(admin, missing)
	assertConnectError(t, err, connect.CodeNotFound, "")
	assertConnectError(t, a.resetSignInLock(admin, missing), connect.CodeNotFound, "")
	_, err = a.signInLock(admin, "42")
	assertConnectError(t, err, connect.CodeNotFound, "")
	assertFieldViolation(t, a.resetSignInLock(admin, "42"), "user_id", "INVALID")
}
