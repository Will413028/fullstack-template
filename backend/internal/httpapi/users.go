package httpapi

import (
	"context"

	"github.com/google/uuid"

	"github.com/Will413028/fullstack-template/backend/internal/httpapi/gen"
	"github.com/Will413028/fullstack-template/backend/internal/modules/auth"
)

// The auth operations: accounts (/users) and sessions (/sessions).

func toUser(u auth.User) gen.User {
	return gen.User{Id: u.ID, Account: u.Account, Role: gen.UserRole(u.Role), IsDisabled: u.IsDisabled, CreatedAt: u.CreatedAt.UTC()}
}

// toSession shows a session of u. current is the id of the session the request carries.
func toSession(sess auth.SessionInfo, u auth.User, current uuid.UUID) gen.Session {
	return gen.Session{
		Id: sess.ID, User: toUser(u), CreatedAt: sess.CreatedAt.UTC(), LastUsedAt: sess.LastUsedAt.UTC(),
		ExpiresAt: sess.ExpiresAt.UTC(), Current: sess.ID == current,
	}
}

func (s *apiServer) CreateUser(ctx context.Context, req gen.CreateUserRequestObject) (gen.CreateUserResponseObject, error) {
	u, sess, err := s.auth.CreateUser(ctx, req.Body.Account, req.Body.Password)
	if err != nil {
		return nil, err
	}
	location, cookie := "/api/users/"+u.ID.String(), s.cookie.set(sess)
	return gen.CreateUser201JSONResponse{
		Body:    toUser(u),
		Headers: gen.CreateUser201ResponseHeaders{Location: &location, SetCookie: &cookie},
	}, nil
}

func (s *apiServer) GetCurrentUser(ctx context.Context, _ gen.GetCurrentUserRequestObject) (gen.GetCurrentUserResponseObject, error) {
	u, ok := currentUser(ctx)
	if !ok {
		return nil, auth.ErrSessionInvalid
	}
	return gen.GetCurrentUser200JSONResponse(toUser(u)), nil
}

// ListUsers is admin-only (requireRole).
func (s *apiServer) ListUsers(ctx context.Context, req gen.ListUsersRequestObject) (gen.ListUsersResponseObject, error) {
	page, err := s.auth.ListUsers(ctx, pageSize(req.Params.Limit), req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	resp := gen.ListUsers200JSONResponse{Users: make([]gen.User, 0, len(page.Users)), NextCursor: nextCursor(page.NextCursor)}
	for _, u := range page.Users {
		resp.Users = append(resp.Users, toUser(u))
	}
	return resp, nil
}

// GetUser serves the user themselves and admins; the auth service answers 404 to anyone else.
func (s *apiServer) GetUser(ctx context.Context, req gen.GetUserRequestObject) (gen.GetUserResponseObject, error) {
	u, err := s.auth.User(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.GetUser200JSONResponse(toUser(u)), nil
}

// PatchUser is admin-only (requireRole). The patch has one field; an empty patch returns the user unchanged.
func (s *apiServer) PatchUser(ctx context.Context, req gen.PatchUserRequestObject) (gen.PatchUserResponseObject, error) {
	var u auth.User
	var err error
	if req.Body.IsDisabled == nil {
		u, err = s.auth.User(ctx, req.Id)
	} else {
		u, err = s.auth.SetDisabled(ctx, req.Id, *req.Body.IsDisabled)
	}
	if err != nil {
		return nil, err
	}
	return gen.PatchUser200JSONResponse(toUser(u)), nil
}

func (s *apiServer) CreateSession(ctx context.Context, req gen.CreateSessionRequestObject) (gen.CreateSessionResponseObject, error) {
	u, sess, err := s.auth.SignIn(ctx, req.Body.Account, req.Body.Password, sessionTokenFrom(ctx))
	if err != nil {
		return nil, err
	}
	location, cookie := "/api/sessions/"+sess.ID.String(), s.cookie.set(sess)
	return gen.CreateSession201JSONResponse{
		Body:    toSession(sess.SessionInfo, u, sess.ID),
		Headers: gen.CreateSession201ResponseHeaders{Location: &location, SetCookie: &cookie},
	}, nil
}

func (s *apiServer) DeleteCurrentSession(ctx context.Context, _ gen.DeleteCurrentSessionRequestObject) (gen.DeleteCurrentSessionResponseObject, error) {
	if err := s.auth.SignOut(ctx, sessionTokenFrom(ctx)); err != nil {
		return nil, err
	}
	cookie := s.cookie.clear()
	return gen.DeleteCurrentSession204Response{Headers: gen.DeleteCurrentSession204ResponseHeaders{SetCookie: &cookie}}, nil
}

func (s *apiServer) DeleteAllSessions(ctx context.Context, _ gen.DeleteAllSessionsRequestObject) (gen.DeleteAllSessionsResponseObject, error) {
	u, ok := currentUser(ctx)
	if !ok {
		return nil, auth.ErrSessionInvalid
	}
	if err := s.auth.SignOutAll(ctx, u.ID); err != nil {
		return nil, err
	}
	cookie := s.cookie.clear()
	return gen.DeleteAllSessions204Response{Headers: gen.DeleteAllSessions204ResponseHeaders{SetCookie: &cookie}}, nil
}

func (s *apiServer) ListSessions(ctx context.Context, req gen.ListSessionsRequestObject) (gen.ListSessionsResponseObject, error) {
	u, ok := currentUser(ctx)
	if !ok {
		return nil, auth.ErrSessionInvalid
	}
	page, err := s.auth.Sessions(ctx, pageSize(req.Params.Limit), req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	current := currentSessionID(ctx)
	resp := gen.ListSessions200JSONResponse{
		Sessions: make([]gen.Session, 0, len(page.Sessions)), NextCursor: nextCursor(page.NextCursor),
	}
	for _, sess := range page.Sessions {
		resp.Sessions = append(resp.Sessions, toSession(sess, u, current))
	}
	return resp, nil
}

func (s *apiServer) GetSession(ctx context.Context, req gen.GetSessionRequestObject) (gen.GetSessionResponseObject, error) {
	u, ok := currentUser(ctx)
	if !ok {
		return nil, auth.ErrSessionInvalid
	}
	sess, err := s.auth.Session(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.GetSession200JSONResponse(toSession(sess, u, currentSessionID(ctx))), nil
}

// GetCurrentSession is GetSession for the session the request carries, as /users/me is for the user.
func (s *apiServer) GetCurrentSession(ctx context.Context, _ gen.GetCurrentSessionRequestObject) (gen.GetCurrentSessionResponseObject, error) {
	u, ok := currentUser(ctx)
	if !ok {
		return nil, auth.ErrSessionInvalid
	}
	current := currentSessionID(ctx)
	sess, err := s.auth.Session(ctx, current)
	if err != nil {
		return nil, err
	}
	return gen.GetCurrentSession200JSONResponse(toSession(sess, u, current)), nil
}

// DeleteSession revokes one of the caller's sessions; revoking the one the request carries also clears the cookie.
func (s *apiServer) DeleteSession(ctx context.Context, req gen.DeleteSessionRequestObject) (gen.DeleteSessionResponseObject, error) {
	if err := s.auth.RevokeSession(ctx, req.Id); err != nil {
		return nil, err
	}
	var resp gen.DeleteSession204Response
	if req.Id == currentSessionID(ctx) {
		cookie := s.cookie.clear()
		resp.Headers.SetCookie = &cookie
	}
	return resp, nil
}
