package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/google/uuid"

	"github.com/Will413028/fullstack-template/backend/internal/modules/auth"
	"github.com/Will413028/fullstack-template/backend/internal/platform/authctx"
)

// Authentication (docs/adr/2026-10-18-api-and-auth-baseline.md D4): the session token lives only in an HttpOnly
// cookie. Which operations need a session is declared once, in the contract's security: the validator calls
// requireSession for those, before it checks parameters and body, so an anonymous request gets 401 rather than a 422
// about its body.

// Cookie names the session cookie and the device cookie (a browser trusted to sign in, auth.Device). Secure is false
// only for local http development, where the names drop the __Host- prefix (it requires Secure);
// config.Config.CookieName and DeviceCookieName decide them.
type Cookie struct {
	Name       string
	DeviceName string
	Secure     bool
}

type sessionTokenKey struct{}

type deviceTokenKey struct{}

// deviceTokenFrom returns the device cookie the request carried, "" without one.
func deviceTokenFrom(ctx context.Context) string {
	v, _ := ctx.Value(deviceTokenKey{}).(string)
	return v
}

type currentUserKey struct{}

type currentSessionKey struct{}

// sessionTokenFrom returns the session token the request carried, valid or not, for sign-out and sign-in.
func sessionTokenFrom(ctx context.Context) string {
	v, _ := ctx.Value(sessionTokenKey{}).(string)
	return v
}

// currentUser returns the signed-in user; the validator guarantees one for operations that declare security.
func currentUser(ctx context.Context) (auth.User, bool) {
	u, ok := ctx.Value(currentUserKey{}).(auth.User)
	return u, ok
}

// currentSessionID returns the id of the valid session the request carries, uuid.Nil when there is none.
func currentSessionID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(currentSessionKey{}).(uuid.UUID)
	return id
}

// authenticate resolves the session cookie before validation. A missing or invalid session leaves the request
// anonymous: operations without security (sign-in, sign-up, sign-out) must work with a stale cookie. An invalid
// cookie is also cleared on the response: it is HttpOnly, so only the server can remove it, and the frontend treats
// a present cookie as signed in (wiki tech/login-death-loop-stale-cookie). A handler that sets a new cookie replaces
// that header. A database error is a 500.
func authenticate(sessions *auth.Service, cookie Cookie, logger *slog.Logger, next http.Handler) http.Handler {
	if sessions == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if d, err := r.Cookie(cookie.DeviceName); err == nil && d.Value != "" {
			ctx = context.WithValue(ctx, deviceTokenKey{}, d.Value)
		}
		c, err := r.Cookie(cookie.Name)
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		ctx = context.WithValue(ctx, sessionTokenKey{}, c.Value)
		u, sessionID, err := sessions.Authenticate(ctx, c.Value)
		switch {
		case err == nil:
			ctx = context.WithValue(ctx, currentUserKey{}, u)
			ctx = context.WithValue(ctx, currentSessionKey{}, sessionID)
			ctx = authctx.With(ctx, authctx.User{ID: u.ID, Role: u.Role})
		case errors.Is(err, auth.ErrSessionInvalid):
			w.Header().Set("Set-Cookie", cookie.clear())
		default:
			writeError(w, r, logger, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

var errNoSession = errors.New("no valid session")

// requireSession is the validator's AuthenticationFunc for the sessionCookie scheme.
func requireSession(ctx context.Context, _ *openapi3filter.AuthenticationInput) error {
	if _, ok := authctx.From(ctx); !ok {
		return errNoSession
	}
	return nil
}

// set returns the Set-Cookie value for a new session: SameSite=Lax and Max-Age the session's absolute lifetime.
func (c Cookie) set(sess auth.Session) string {
	return c.cookie(c.Name, sess.Token, int(sess.MaxAge.Seconds()), http.SameSiteLaxMode)
}

// device returns the Set-Cookie value for a newly trusted browser: SameSite=Strict, since only a same-site sign-in
// reads it, and Max-Age the device's lifetime.
func (c Cookie) device(d *auth.Device) string {
	return c.cookie(c.DeviceName, d.Token, int(d.MaxAge.Seconds()), http.SameSiteStrictMode)
}

// clear expires the session cookie (Max-Age=0).
func (c Cookie) clear() string {
	return c.cookie(c.Name, "", -1, http.SameSiteLaxMode)
}

// cookie is every cookie the API sets: HttpOnly so scripts cannot read it, Path=/ and no Domain (the __Host- prefix
// requires both), Secure unless COOKIE_SECURE=false for local development over http.
func (c Cookie) cookie(name, value string, maxAge int, sameSite http.SameSite) string {
	return (&http.Cookie{ //nolint:gosec // Secure comes from COOKIE_SECURE, false only for local http
		Name: name, Value: value, Path: "/", MaxAge: maxAge, HttpOnly: true, Secure: c.Secure, SameSite: sameSite,
	}).String()
}
