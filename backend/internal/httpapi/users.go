package httpapi

import (
	"context"

	"github.com/Will413028/fullstack-template/backend/internal/httpapi/gen"
	"github.com/Will413028/fullstack-template/backend/internal/modules/auth"
)

// The auth operations: accounts (/users) and sessions (/sessions).

func toUser(u auth.User) gen.User {
	return gen.User{Id: u.ID, Account: u.Account, Role: gen.UserRole(u.Role), IsDisabled: u.IsDisabled, CreatedAt: u.CreatedAt.UTC()}
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
	location, cookie := "/api/sessions/current", s.cookie.set(sess)
	return gen.CreateSession201JSONResponse{
		Body:    toUser(u),
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
