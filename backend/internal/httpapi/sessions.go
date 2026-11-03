package httpapi

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionsv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/sessions/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/gen/app/sessions/v1/sessionsv1connect"
	"github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc"
	"github.com/Will413028/template-go-nextjs/backend/internal/modules/auth"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/apperr"
)

// The session services, app.sessions.v1: signing in and out (Public, so they work with a stale cookie) and the
// signed-in user's sessions (App). The auth service throttles sign-ins and scopes every query to the caller; these
// handlers map the messages and set or clear the cookies.

// toSession shows a session of u. current is the id of the session the request carries.
func toSession(sess auth.SessionInfo, u auth.User, current uuid.UUID) *sessionsv1.Session {
	return &sessionsv1.Session{
		Id: sess.ID.String(), User: toUser(u), CreateTime: timestamppb.New(sess.CreatedAt),
		LastUseTime: timestamppb.New(sess.LastUsedAt), ExpireTime: timestamppb.New(sess.ExpiresAt),
		Current: sess.ID == current,
	}
}

func sessionServices(svc *auth.Service, cookie Cookie) []rpc.Service {
	return []rpc.Service{
		{
			Desc: sessionsv1.File_app_sessions_v1_sessions_proto.Services().ByName("SessionPublicService"),
			New: func(opts ...connect.HandlerOption) (string, http.Handler) {
				return sessionsv1connect.NewSessionPublicServiceHandler(sessionPublicHandler{auth: svc, cookie: cookie}, opts...)
			},
		},
		{
			Desc: sessionsv1.File_app_sessions_v1_sessions_proto.Services().ByName("SessionAppService"),
			New: func(opts ...connect.HandlerOption) (string, http.Handler) {
				return sessionsv1connect.NewSessionAppServiceHandler(sessionAppHandler{auth: svc, cookie: cookie}, opts...)
			},
		},
	}
}

// sessionPublicHandler serves app.sessions.v1.SessionPublicService.
type sessionPublicHandler struct {
	auth   *auth.Service
	cookie Cookie
}

// CreateSession signs in with the client's address and trusted-device cookie (authenticate and withRequestInfo put
// them in the context), and ends the session the request carried. It sets the new session's cookie and, for a
// browser the account does not trust yet, the device cookie: two Set-Cookie headers. The entry limits it per client
// address (connectLimited); the auth service throttles it per account and client.
func (h sessionPublicHandler) CreateSession(ctx context.Context, req *connect.Request[sessionsv1.CreateSessionRequest]) (*connect.Response[sessionsv1.CreateSessionResponse], error) {
	// withRequestInfo always finds an address on a TCP listener; without one (a unix socket) every client would share
	// the "unknown" bucket, so serve listens on TCP only.
	address := "unknown"
	if ip := clientIPFrom(ctx); ip.IsValid() {
		address = limitKey(ip).String()
	}
	client := auth.Client{Address: address, DeviceToken: deviceTokenFrom(ctx)}
	u, sess, err := h.auth.SignIn(ctx, req.Msg.GetAccount(), req.Msg.GetPassword(), sessionTokenFrom(ctx), client)
	if err != nil {
		return nil, err
	}
	resp := connect.NewResponse(&sessionsv1.CreateSessionResponse{Session: toSession(sess.SessionInfo, u, sess.ID)})
	resp.Header().Add("Set-Cookie", h.cookie.set(sess))
	if sess.Device != nil {
		resp.Header().Add("Set-Cookie", h.cookie.device(sess.Device))
	}
	return resp, nil
}

// DeleteCurrentSession signs out the session the request carries, valid or not, and clears the cookie.
func (h sessionPublicHandler) DeleteCurrentSession(ctx context.Context, _ *connect.Request[sessionsv1.DeleteCurrentSessionRequest]) (*connect.Response[sessionsv1.DeleteCurrentSessionResponse], error) {
	if err := h.auth.SignOut(ctx, sessionTokenFrom(ctx)); err != nil {
		return nil, err
	}
	resp := connect.NewResponse(&sessionsv1.DeleteCurrentSessionResponse{})
	resp.Header().Add("Set-Cookie", h.cookie.clear())
	return resp, nil
}

// sessionAppHandler serves app.sessions.v1.SessionAppService.
type sessionAppHandler struct {
	auth   *auth.Service
	cookie Cookie
}

func (h sessionAppHandler) ListSessions(ctx context.Context, req *connect.Request[sessionsv1.ListSessionsRequest]) (*connect.Response[sessionsv1.ListSessionsResponse], error) {
	u, err := signedIn(ctx)
	if err != nil {
		return nil, err
	}
	size, cursor, err := pageRequest(req.Msg.GetPageSize(), req.Msg.GetPageToken())
	if err != nil {
		return nil, err
	}
	page, err := h.auth.Sessions(ctx, size, cursor)
	if err != nil {
		return nil, err
	}
	current := currentSessionID(ctx)
	resp := &sessionsv1.ListSessionsResponse{
		Sessions: make([]*sessionsv1.Session, 0, len(page.Sessions)), NextPageToken: page.NextCursor,
	}
	for _, sess := range page.Sessions {
		resp.Sessions = append(resp.Sessions, toSession(sess, u, current))
	}
	return connect.NewResponse(resp), nil
}

func (h sessionAppHandler) GetSession(ctx context.Context, req *connect.Request[sessionsv1.GetSessionRequest]) (*connect.Response[sessionsv1.GetSessionResponse], error) {
	id, err := rpc.ParseID("id", req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	u, err := signedIn(ctx)
	if err != nil {
		return nil, err
	}
	sess, err := h.auth.Session(ctx, id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&sessionsv1.GetSessionResponse{Session: toSession(sess, u, currentSessionID(ctx))}), nil
}

// GetCurrentSession is GetSession for the session the request carries, as GetCurrentUser is for the user.
func (h sessionAppHandler) GetCurrentSession(ctx context.Context, _ *connect.Request[sessionsv1.GetCurrentSessionRequest]) (*connect.Response[sessionsv1.GetCurrentSessionResponse], error) {
	u, err := signedIn(ctx)
	if err != nil {
		return nil, err
	}
	current := currentSessionID(ctx)
	sess, err := h.auth.Session(ctx, current)
	if e, ok := apperr.As(err); ok && e.Kind == apperr.NotFound {
		// revoked or expired since authenticate read it: the request no longer has a session
		return nil, auth.ErrSessionInvalid
	}
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&sessionsv1.GetCurrentSessionResponse{Session: toSession(sess, u, current)}), nil
}

// DeleteSession revokes one of the caller's sessions; revoking the one the request carries also clears the cookie.
func (h sessionAppHandler) DeleteSession(ctx context.Context, req *connect.Request[sessionsv1.DeleteSessionRequest]) (*connect.Response[sessionsv1.DeleteSessionResponse], error) {
	id, err := rpc.ParseID("id", req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if err := h.auth.RevokeSession(ctx, id); err != nil {
		return nil, err
	}
	resp := connect.NewResponse(&sessionsv1.DeleteSessionResponse{})
	if id == currentSessionID(ctx) {
		resp.Header().Add("Set-Cookie", h.cookie.clear())
	}
	return resp, nil
}

// DeleteAllSessions signs the caller out everywhere, forgets their trusted devices and clears the cookie.
func (h sessionAppHandler) DeleteAllSessions(ctx context.Context, _ *connect.Request[sessionsv1.DeleteAllSessionsRequest]) (*connect.Response[sessionsv1.DeleteAllSessionsResponse], error) {
	u, err := signedIn(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.auth.SignOutAll(ctx, u.ID); err != nil {
		return nil, err
	}
	resp := connect.NewResponse(&sessionsv1.DeleteAllSessionsResponse{})
	resp.Header().Add("Set-Cookie", h.cookie.clear())
	return resp, nil
}
