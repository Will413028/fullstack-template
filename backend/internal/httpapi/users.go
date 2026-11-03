package httpapi

import (
	"context"
	"math"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminusersv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/admin/users/v1"
	adminusersv1connect "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/admin/users/v1/usersv1connect"
	usersv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/users/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/gen/app/users/v1/usersv1connect"
	"github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc"
	"github.com/Will413028/template-go-nextjs/backend/internal/modules/auth"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/apperr"
)

// The account services: app.users.v1 (sign-up, the signed-in user) and app.admin.users.v1 (account management). The
// auth service checks the limits, the throttles and who may see whom; these handlers map the messages, the field paths
// and the cookies. A handler of an App or Admin service always has a signed-in user: connectEntry refuses anonymous
// calls before decoding, and the admin interceptor refuses non-admins.

// Page sizes of ListUsers and ListSessions, as the protos document them.
const (
	defaultAuthPage = 20
	maxAuthPage     = 100
)

// userWritable are the User fields UserAdminService.UpdateUser may set.
var userWritable = []string{"disabled"}

// roles maps the auth module's roles to the proto's. users.role's CHECK allows only these (TestRolesMapped).
var roles = map[string]usersv1.Role{auth.RoleAdmin: usersv1.Role_ROLE_ADMIN, auth.RoleUser: usersv1.Role_ROLE_USER}

// errUserNotFound is UserAppService.GetUser's answer for any id but the caller's: the same as a user that does not
// exist.
var errUserNotFound = apperr.New(apperr.NotFound, "", "user not found")

func toUser(u auth.User) *usersv1.User {
	return &usersv1.User{
		Id: u.ID.String(), Account: u.Account, Role: roles[u.Role], Disabled: proto.Bool(u.IsDisabled),
		CreateTime: timestamppb.New(u.CreatedAt),
	}
}

// signedIn returns the signed-in user, for App and Admin handlers.
func signedIn(ctx context.Context) (auth.User, error) {
	u, ok := currentUser(ctx)
	if !ok {
		return auth.User{}, auth.ErrSessionInvalid
	}
	return u, nil
}

func userServices(svc *auth.Service, cookie Cookie) []rpc.Service {
	return []rpc.Service{
		{
			Desc: usersv1.File_app_users_v1_users_proto.Services().ByName("UserPublicService"),
			New: func(opts ...connect.HandlerOption) (string, http.Handler) {
				return usersv1connect.NewUserPublicServiceHandler(userPublicHandler{auth: svc, cookie: cookie}, opts...)
			},
		},
		{
			Desc: usersv1.File_app_users_v1_users_proto.Services().ByName("UserAppService"),
			New: func(opts ...connect.HandlerOption) (string, http.Handler) {
				return usersv1connect.NewUserAppServiceHandler(userAppHandler{auth: svc, cookie: cookie}, opts...)
			},
		},
		{
			Desc: adminusersv1.File_app_admin_users_v1_users_proto.Services().ByName("UserAdminService"),
			New: func(opts ...connect.HandlerOption) (string, http.Handler) {
				return adminusersv1connect.NewUserAdminServiceHandler(userAdminHandler{auth: svc}, opts...)
			},
		},
	}
}

// userPublicHandler serves app.users.v1.UserPublicService.
type userPublicHandler struct {
	auth   *auth.Service
	cookie Cookie
}

// CreateUser signs the new account in: the response sets the session cookie. The entry limits it per client address
// (connectLimited).
func (h userPublicHandler) CreateUser(ctx context.Context, req *connect.Request[usersv1.CreateUserRequest]) (*connect.Response[usersv1.CreateUserResponse], error) {
	u, sess, err := h.auth.CreateUser(ctx, req.Msg.GetAccount(), req.Msg.GetPassword())
	if err != nil {
		return nil, err
	}
	resp := connect.NewResponse(&usersv1.CreateUserResponse{User: toUser(u)})
	resp.Header().Add("Set-Cookie", h.cookie.set(sess))
	return resp, nil
}

// userAppHandler serves app.users.v1.UserAppService.
type userAppHandler struct {
	auth   *auth.Service
	cookie Cookie
}

func (h userAppHandler) GetCurrentUser(ctx context.Context, _ *connect.Request[usersv1.GetCurrentUserRequest]) (*connect.Response[usersv1.GetCurrentUserResponse], error) {
	u, err := signedIn(ctx)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&usersv1.GetCurrentUserResponse{User: toUser(u)}), nil
}

// DeleteCurrentUser deletes the caller's account and clears the cookie; the service requires a recent sign-in.
func (h userAppHandler) DeleteCurrentUser(ctx context.Context, _ *connect.Request[usersv1.DeleteCurrentUserRequest]) (*connect.Response[usersv1.DeleteCurrentUserResponse], error) {
	if err := h.auth.DeleteOwnAccount(ctx, currentSessionID(ctx)); err != nil {
		return nil, err
	}
	resp := connect.NewResponse(&usersv1.DeleteCurrentUserResponse{})
	resp.Header().Add("Set-Cookie", h.cookie.clear())
	return resp, nil
}

// GetUser serves the caller only; admins read other users through UserAdminService.GetUser.
func (h userAppHandler) GetUser(ctx context.Context, req *connect.Request[usersv1.GetUserRequest]) (*connect.Response[usersv1.GetUserResponse], error) {
	id, err := rpc.ParseID("id", req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	u, err := signedIn(ctx)
	if err != nil {
		return nil, err
	}
	if id != u.ID {
		return nil, errUserNotFound
	}
	return connect.NewResponse(&usersv1.GetUserResponse{User: toUser(u)}), nil
}

// userAdminHandler serves app.admin.users.v1.UserAdminService; the admin interceptor has checked the role.
type userAdminHandler struct {
	auth *auth.Service
}

func (h userAdminHandler) ListUsers(ctx context.Context, req *connect.Request[adminusersv1.ListUsersRequest]) (*connect.Response[adminusersv1.ListUsersResponse], error) {
	size, cursor, err := rpc.PageRequest(req.Msg.GetPageSize(), req.Msg.GetPageToken(), defaultAuthPage, maxAuthPage)
	if err != nil {
		return nil, err
	}
	page, err := h.auth.ListUsers(ctx, size, cursor)
	if err != nil {
		return nil, err
	}
	resp := &adminusersv1.ListUsersResponse{Users: make([]*usersv1.User, 0, len(page.Users)), NextPageToken: page.NextCursor}
	for _, u := range page.Users {
		resp.Users = append(resp.Users, toUser(u))
	}
	return connect.NewResponse(resp), nil
}

func (h userAdminHandler) GetUser(ctx context.Context, req *connect.Request[adminusersv1.GetUserRequest]) (*connect.Response[adminusersv1.GetUserResponse], error) {
	id, err := rpc.ParseID("id", req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	u, err := h.auth.User(ctx, id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminusersv1.GetUserResponse{User: toUser(u)}), nil
}

// UpdateUser changes disabled when the mask lists it (or, without a mask, when it is set); otherwise it returns the
// user unchanged.
func (h userAdminHandler) UpdateUser(ctx context.Context, req *connect.Request[adminusersv1.UpdateUserRequest]) (*connect.Response[adminusersv1.UpdateUserResponse], error) {
	in := req.Msg.GetUser()
	if in == nil {
		in = &usersv1.User{}
	}
	id, err := rpc.ParseID("id", in.GetId())
	if err != nil {
		return nil, rpc.Under("user", err)
	}
	paths, err := rpc.UpdatePaths(req.Msg.GetUpdateMask(), in, userWritable...)
	if err != nil {
		return nil, err
	}
	var u auth.User
	if paths["disabled"] {
		u, err = h.auth.SetDisabled(ctx, id, in.GetDisabled())
	} else {
		u, err = h.auth.User(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminusersv1.UpdateUserResponse{User: toUser(u)}), nil
}

// DeleteUser deletes an account; the service requires the admin's recent sign-in.
func (h userAdminHandler) DeleteUser(ctx context.Context, req *connect.Request[adminusersv1.DeleteUserRequest]) (*connect.Response[adminusersv1.DeleteUserResponse], error) {
	id, err := rpc.ParseID("id", req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if err := h.auth.DeleteUser(ctx, currentSessionID(ctx), id); err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminusersv1.DeleteUserResponse{}), nil
}

func (h userAdminHandler) GetSignInLock(ctx context.Context, req *connect.Request[adminusersv1.GetSignInLockRequest]) (*connect.Response[adminusersv1.GetSignInLockResponse], error) {
	id, err := rpc.ParseID("user_id", req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	lock, err := h.auth.SignInLock(ctx, id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminusersv1.GetSignInLockResponse{SignInLock: &adminusersv1.SignInLock{
		Locked: lock.Locked, Failures: clampInt32(lock.Failures),
	}}), nil
}

func (h userAdminHandler) ResetSignInLock(ctx context.Context, req *connect.Request[adminusersv1.ResetSignInLockRequest]) (*connect.Response[adminusersv1.ResetSignInLockResponse], error) {
	id, err := rpc.ParseID("user_id", req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	if err := h.auth.UnlockSignIn(ctx, id); err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminusersv1.ResetSignInLockResponse{}), nil
}

// clampInt32 is n within int32's range. A sign-in lock's failure count stops growing long before it (the lock starts at
// 100), but the column is a bigint.
func clampInt32(n int64) int32 {
	switch {
	case n > math.MaxInt32:
		return math.MaxInt32
	case n < math.MinInt32:
		return math.MinInt32
	}
	return int32(n)
}
