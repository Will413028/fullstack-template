package httpapi

import (
	"bytes"
	"net/http"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	adminusersv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/admin/users/v1"
	adminusersv1connect "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/admin/users/v1/usersv1connect"
	sessionsv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/sessions/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/gen/app/sessions/v1/sessionsv1connect"
	usersv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/users/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/gen/app/users/v1/usersv1connect"
)

// The generated clients of the user and session services, as the frontend calls them: same origin, reads with GET.
// token is the session cookie ("" for none); opts change each request (requestOpt).

// requestOpt changes a test client's request before the handler serves it.
type requestOpt func(*http.Request)

// fromPeer sends the request from addr (host:port), the peer address withRequestInfo reads.
func fromPeer(addr string) requestOpt { return func(r *http.Request) { r.RemoteAddr = addr } }

// withCookie adds a cookie, for example the device cookie.
func withCookie(name, value string) requestOpt {
	return func(r *http.Request) { r.Header.Add("Cookie", name+"="+value) }
}

// withHeader sets a request header, for example Sec-Fetch-Site or X-Forwarded-For.
func withHeader(key, value string) requestOpt {
	return func(r *http.Request) { r.Header.Set(key, value) }
}

func (a *authAPI) client(token string, opts ...requestOpt) *http.Client {
	return &http.Client{Transport: handlerTransport{h: a.h, cookie: token, opts: opts}}
}

func (a *authAPI) publicUsers(token string, opts ...requestOpt) usersv1connect.UserPublicServiceClient {
	return usersv1connect.NewUserPublicServiceClient(a.client(token, opts...), baseURL, connect.WithHTTPGet())
}

func (a *authAPI) users(token string, opts ...requestOpt) usersv1connect.UserAppServiceClient {
	return usersv1connect.NewUserAppServiceClient(a.client(token, opts...), baseURL, connect.WithHTTPGet())
}

func (a *authAPI) publicSessions(token string, opts ...requestOpt) sessionsv1connect.SessionPublicServiceClient {
	return sessionsv1connect.NewSessionPublicServiceClient(a.client(token, opts...), baseURL, connect.WithHTTPGet())
}

func (a *authAPI) sessions(token string, opts ...requestOpt) sessionsv1connect.SessionAppServiceClient {
	return sessionsv1connect.NewSessionAppServiceClient(a.client(token, opts...), baseURL, connect.WithHTTPGet())
}

func (a *authAPI) admin(token string, opts ...requestOpt) adminusersv1connect.UserAdminServiceClient {
	return adminusersv1connect.NewUserAdminServiceClient(a.client(token, opts...), baseURL, connect.WithHTTPGet())
}

// signUp calls CreateUser without a session.
func (a *authAPI) signUp(account, password string, opts ...requestOpt) (*connect.Response[usersv1.CreateUserResponse], error) {
	a.t.Helper()
	return a.publicUsers("", opts...).CreateUser(a.t.Context(), connect.NewRequest(&usersv1.CreateUserRequest{
		Account: account, Password: password,
	}))
}

// signIn calls CreateSession, carrying token as the session cookie when it is not empty.
func (a *authAPI) signIn(account, password, token string, opts ...requestOpt) (*connect.Response[sessionsv1.CreateSessionResponse], error) {
	a.t.Helper()
	return a.publicSessions(token, opts...).CreateSession(a.t.Context(), connect.NewRequest(&sessionsv1.CreateSessionRequest{
		Account: account, Password: password,
	}))
}

// me calls GetCurrentUser with the session token.
func (a *authAPI) me(token string) (*usersv1.User, error) {
	a.t.Helper()
	resp, err := a.users(token).GetCurrentUser(a.t.Context(), connect.NewRequest(&usersv1.GetCurrentUserRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetUser(), nil
}

// responseCookies are the cookies a response's headers set (a Connect error's headers are its Meta).
func responseCookies(h http.Header) []*http.Cookie {
	return (&http.Response{Header: h}).Cookies()
}

// sessionCookie returns the one session cookie h sets (a sign-in may also set the device cookie).
func (a *authAPI) sessionCookie(h http.Header) *http.Cookie {
	a.t.Helper()
	var found []*http.Cookie
	for _, c := range responseCookies(h) {
		if c.Name == cookieName {
			found = append(found, c)
		}
	}
	if len(found) != 1 {
		a.t.Fatalf("Set-Cookie = %v, want exactly one %s", h.Values("Set-Cookie"), cookieName)
	}
	return found[0]
}

// mustSignUp creates an account and returns its user and session token.
func (a *authAPI) mustSignUp(account string) (*usersv1.User, string) {
	a.t.Helper()
	resp, err := a.signUp(account, goodPassword)
	if err != nil {
		a.t.Fatalf("sign up %s: %v", account, err)
	}
	return resp.Msg.GetUser(), a.sessionCookie(resp.Header()).Value
}

func (a *authAPI) mustSignIn(account, password string) string {
	a.t.Helper()
	resp, err := a.signIn(account, password, "")
	if err != nil {
		a.t.Fatalf("sign in %s: %v", account, err)
	}
	return a.sessionCookie(resp.Header()).Value
}

// mustAdmin creates an admin the way seed-admin does and signs it in.
func (a *authAPI) mustAdmin() string {
	a.t.Helper()
	if _, err := a.svc.EnsureAdmin(a.t.Context(), "root", goodPassword); err != nil {
		a.t.Fatal(err)
	}
	return a.mustSignIn("root", goodPassword)
}

// getUser calls UserAppService.GetUser (the caller's own account only).
func (a *authAPI) getUser(token, id string) (*usersv1.User, error) {
	a.t.Helper()
	resp, err := a.users(token).GetUser(a.t.Context(), connect.NewRequest(&usersv1.GetUserRequest{Id: id}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetUser(), nil
}

// adminGetUser calls UserAdminService.GetUser (any account).
func (a *authAPI) adminGetUser(token, id string) (*usersv1.User, error) {
	a.t.Helper()
	resp, err := a.admin(token).GetUser(a.t.Context(), connect.NewRequest(&adminusersv1.GetUserRequest{Id: id}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetUser(), nil
}

// updateUser calls UserAdminService.UpdateUser with a mask of paths; no paths sends no mask.
func (a *authAPI) updateUser(token string, user *usersv1.User, paths ...string) (*usersv1.User, error) {
	a.t.Helper()
	req := &adminusersv1.UpdateUserRequest{User: user}
	if paths != nil {
		req.UpdateMask = &fieldmaskpb.FieldMask{Paths: paths}
	}
	resp, err := a.admin(token).UpdateUser(a.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetUser(), nil
}

// deleteUser calls UserAdminService.DeleteUser.
func (a *authAPI) deleteUser(token, id string) error {
	a.t.Helper()
	_, err := a.admin(token).DeleteUser(a.t.Context(), connect.NewRequest(&adminusersv1.DeleteUserRequest{Id: id}))
	return err
}

// deleteCurrentUser calls UserAppService.DeleteCurrentUser.
func (a *authAPI) deleteCurrentUser(token string) (*connect.Response[usersv1.DeleteCurrentUserResponse], error) {
	a.t.Helper()
	return a.users(token).DeleteCurrentUser(a.t.Context(), connect.NewRequest(&usersv1.DeleteCurrentUserRequest{}))
}

// signOut calls DeleteCurrentSession, carrying token as the session cookie when it is not empty.
func (a *authAPI) signOut(token string) (*connect.Response[sessionsv1.DeleteCurrentSessionResponse], error) {
	a.t.Helper()
	return a.publicSessions(token).DeleteCurrentSession(a.t.Context(), connect.NewRequest(&sessionsv1.DeleteCurrentSessionRequest{}))
}

// currentSession returns the session token authenticates (GetCurrentSession).
func (a *authAPI) currentSession(token string) *sessionsv1.Session {
	a.t.Helper()
	resp, err := a.sessions(token).GetCurrentSession(a.t.Context(), connect.NewRequest(&sessionsv1.GetCurrentSessionRequest{}))
	if err != nil {
		a.t.Fatalf("GetCurrentSession: %v", err)
	}
	return resp.Msg.GetSession()
}

func (a *authAPI) getSession(token, id string) (*sessionsv1.Session, error) {
	a.t.Helper()
	resp, err := a.sessions(token).GetSession(a.t.Context(), connect.NewRequest(&sessionsv1.GetSessionRequest{Id: id}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetSession(), nil
}

func (a *authAPI) deleteSession(token, id string) (*connect.Response[sessionsv1.DeleteSessionResponse], error) {
	a.t.Helper()
	return a.sessions(token).DeleteSession(a.t.Context(), connect.NewRequest(&sessionsv1.DeleteSessionRequest{Id: id}))
}

// mustSignInSession signs in and returns the new session and its token.
func (a *authAPI) mustSignInSession(account string) (*sessionsv1.Session, string) {
	a.t.Helper()
	resp, err := a.signIn(account, goodPassword, "")
	if err != nil {
		a.t.Fatalf("sign in %s: %v", account, err)
	}
	return resp.Msg.GetSession(), a.sessionCookie(resp.Header()).Value
}

// outcome is "ok" for a call that succeeded and its Connect code otherwise, for counting the results of concurrent
// calls.
func outcome(err error) string {
	if err == nil {
		return "ok"
	}
	return connect.CodeOf(err).String()
}

// assertViolations checks that err is invalid_argument whose BadRequest lists exactly want, in order, each written
// "<field> <REASON>": every invalid field is reported at once.
func assertViolations(t *testing.T, err error, want ...string) {
	t.Helper()
	assertConnectError(t, err, connect.CodeInvalidArgument, "")
	br := errorDetail[errdetails.BadRequest](t, asConnectError(t, err))
	var got []string
	for _, v := range br.GetFieldViolations() {
		got = append(got, v.GetField()+" "+v.GetReason())
	}
	if !slices.Equal(got, want) {
		t.Fatalf("violations = %q, want %q", got, want)
	}
}

// assertSameError checks that two calls failed alike: the same code, message and details, so a client cannot tell
// the two cases apart.
func assertSameError(t *testing.T, got, want error) {
	t.Helper()
	g, w := asConnectError(t, got), asConnectError(t, want)
	same := g.Code() == w.Code() && g.Message() == w.Message() && len(g.Details()) == len(w.Details())
	for i := 0; same && i < len(g.Details()); i++ {
		same = g.Details()[i].Type() == w.Details()[i].Type() && bytes.Equal(g.Details()[i].Bytes(), w.Details()[i].Bytes())
	}
	if !same {
		t.Errorf("error = %v (%d details), want the same as %v (%d details)", g, len(g.Details()), w, len(w.Details()))
	}
}

// assertRetryInfo checks err's RetryInfo: a delay within the second up to want (the old Retry-After, in whole seconds,
// was want).
func assertRetryInfo(t *testing.T, err error, want time.Duration) {
	t.Helper()
	retry := errorDetail[errdetails.RetryInfo](t, asConnectError(t, err))
	if d := retry.GetRetryDelay().AsDuration(); retry == nil || d <= want-time.Second || d > want {
		t.Errorf("RetryInfo = %v, want a delay of %s", retry, want)
	}
}
