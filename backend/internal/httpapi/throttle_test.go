package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Will413028/fullstack-template/backend/internal/modules/auth"
)

const deviceCookieName = "__Host-device"

// signInFrom signs in from the client address ip, with the device cookie when device is not empty.
func (a *authAPI) signInFrom(ip, account, password, device string) *httptest.ResponseRecorder {
	a.t.Helper()
	req := httptest.NewRequestWithContext(a.t.Context(), http.MethodPost, "http://app.test/api/sessions",
		strings.NewReader(credentials(account, password)))
	req.RemoteAddr = ip + ":40000"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if device != "" {
		req.Header.Set("Cookie", deviceCookieName+"="+device)
	}
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	return rec
}

// deviceCookie returns the device cookie rec sets, nil without one.
func deviceCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == deviceCookieName {
			return c
		}
	}
	return nil
}

// lockOut fails n sign-ins for account, each from its own address, so only the account-wide count can stop them.
func (a *authAPI) lockOut(account string, n int) {
	a.t.Helper()
	for i := range n {
		if rec := a.signInFrom(fmt.Sprintf("198.51.%d.%d", i/200, i%200+1), account, "not the password", ""); rec.Code != http.StatusUnauthorized {
			a.t.Fatalf("failure %d = %d %s", i+1, rec.Code, rec.Body)
		}
	}
}

// One client's failures make that client wait; another client signs in to the same account at once.
func TestSignInThrottlePerClient(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	for i := range 5 {
		if rec := a.signInFrom("198.51.100.1", "alice", "not the password", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d", i+1, rec.Code)
		}
	}
	p := assertProblem(t, a.signInFrom("198.51.100.1", "alice", goodPassword, ""), http.StatusTooManyRequests, "/problems/rate-limited")
	assertCode(t, p, auth.CodeLoginThrottled)
	if rec := a.signInFrom("198.51.100.2", "alice", goodPassword, ""); rec.Code != http.StatusCreated {
		t.Errorf("another client = %d %s, want 201", rec.Code, rec.Body)
	}
}

// After 100 failures from clients it does not know, the account refuses them all, but a browser that signed in
// before (device cookie) still gets in, and its sign-in clears the lock (NIST SP 800-63B 3.2.2).
func TestUntrustedClientsLockedTrustedDeviceNot(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	rec := a.signInFrom("203.0.113.1", "alice", goodPassword, "")
	device := deviceCookie(rec)
	if rec.Code != http.StatusCreated || device == nil {
		t.Fatalf("first sign-in = %d, device cookie %v", rec.Code, device)
	}
	if !device.HttpOnly || !device.Secure || device.SameSite != http.SameSiteStrictMode || device.Path != "/" ||
		device.MaxAge != int((180*24*time.Hour).Seconds()) {
		t.Errorf("device cookie = %+v", device)
	}
	a.lockOut("alice", 99)
	if rec := a.signInFrom("192.0.2.10", "alice", goodPassword, ""); rec.Code != http.StatusCreated {
		t.Fatalf("after 99 failures = %d, want 201 (and the count starts over)", rec.Code)
	}
	a.lockOut("alice", 100)
	p := assertProblem(t, a.signInFrom("192.0.2.11", "alice", goodPassword, ""), http.StatusTooManyRequests, "/problems/rate-limited")
	assertCode(t, p, auth.CodeLoginThrottled)
	if rec := a.signInFrom("192.0.2.12", "alice", goodPassword, device.Value); rec.Code != http.StatusCreated {
		t.Fatalf("trusted device = %d %s, want 201", rec.Code, rec.Body)
	}
	if rec := a.signInFrom("192.0.2.13", "alice", goodPassword, ""); rec.Code != http.StatusCreated {
		t.Errorf("after the owner signed in = %d, want 201", rec.Code)
	}
}

// A device cookie of another account does not exempt a client.
func TestDeviceCookieIsPerAccount(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	a.mustSignUp("mallory")
	device := deviceCookie(a.signInFrom("203.0.113.1", "mallory", goodPassword, ""))
	a.lockOut("alice", 100)
	assertProblem(t, a.signInFrom("203.0.113.1", "alice", goodPassword, device.Value), http.StatusTooManyRequests, "/problems/rate-limited")
}

// Signing out everywhere forgets the account's devices too.
func TestSignOutEverywhereForgetsDevices(t *testing.T) {
	a := newAuthAPI(t)
	a.mustSignUp("alice")
	rec := a.signInFrom("203.0.113.1", "alice", goodPassword, "")
	device, token := deviceCookie(rec), sessionCookie(t, rec).Value
	if got := a.do(http.MethodDelete, "/api/sessions", token, "", ""); got.Code != http.StatusNoContent {
		t.Fatalf("sign out everywhere = %d", got.Code)
	}
	a.lockOut("alice", 100)
	assertProblem(t, a.signInFrom("203.0.113.1", "alice", goodPassword, device.Value), http.StatusTooManyRequests, "/problems/rate-limited")
}

// An admin sees and lifts the lock; the unlock is audited.
func TestAdminUnlocksSignIn(t *testing.T) {
	a := newAuthAPI(t)
	admin := a.mustAdmin()
	alice, _ := a.mustSignUp("alice")
	path := "/api/users/" + alice.Id.String() + "/sign-in-lock"
	if rec := a.do(http.MethodGet, path, admin, "", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"locked":false`) {
		t.Errorf("before = %d %s", rec.Code, rec.Body)
	}
	a.lockOut("alice", 100)
	if rec := a.do(http.MethodGet, path, admin, "", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"locked":true`) {
		t.Errorf("locked = %d %s", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodDelete, path, admin, "", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("unlock = %d %s", rec.Code, rec.Body)
	}
	if rec := a.signInFrom("192.0.2.20", "alice", goodPassword, ""); rec.Code != http.StatusCreated {
		t.Errorf("after unlock = %d, want 201", rec.Code)
	}
	events := a.auditEvents()
	if e := events[len(events)-1]; e.action != auth.AuditUserUnlocked || e.target != alice.Id {
		t.Errorf("audit row = %+v, want user.unlocked for alice", e)
	}
}
