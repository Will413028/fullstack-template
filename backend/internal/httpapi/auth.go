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

// Cookie names the session cookie. Secure is false only for local http development, where the name drops the
// __Host- prefix (it requires Secure); config.Config.CookieName decides the name.
type Cookie struct {
	Name   string
	Secure bool
}

type sessionTokenKey struct{}

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
		c, err := r.Cookie(cookie.Name)
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), sessionTokenKey{}, c.Value)
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

// set returns the Set-Cookie value for a new session: HttpOnly so scripts cannot read it, SameSite=Lax, Path=/, no
// Domain, and Max-Age the session's absolute lifetime.
func (c Cookie) set(sess auth.Session) string {
	// Secure is false only with COOKIE_SECURE=false, for local development over http.
	return (&http.Cookie{ //nolint:gosec // see above
		Name: c.Name, Value: sess.Token, Path: "/", MaxAge: int(sess.MaxAge.Seconds()),
		HttpOnly: true, Secure: c.Secure, SameSite: http.SameSiteLaxMode,
	}).String()
}

// clear expires the session cookie (Max-Age=0).
func (c Cookie) clear() string {
	return (&http.Cookie{ //nolint:gosec // as in set
		Name: c.Name, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: c.Secure, SameSite: http.SameSiteLaxMode,
	}).String()
}
