package rpc

import (
	"context"
	"log/slog"
	"runtime/debug"

	"connectrpc.com/connect"

	"github.com/Will413028/template-go-nextjs/backend/internal/modules/auth"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/apperr"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/authctx"
)

// The interceptors run in this order around every handler: convertErrors, recoverPanics, requireAdmin. connect-go
// decodes the request before any interceptor, so checks that must come before decoding (a session for App and Admin
// services, the rate limit) run in the net/http layer instead (internal/httpapi).

// convertErrors turns the handler's error into a Connect error (Convert) and logs what the client does not see.
// Errors connect-go returns before the interceptors (a body it cannot decode, a wrong content type) are 4xx and are
// already in the access log with their procedure and status.
func convertErrors(logger *slog.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			if err == nil {
				return resp, nil
			}
			c := Convert(err)
			LogConverted(ctx, logger, req.Spec().Procedure, err, c)
			return nil, c.Err
		}
	}
}

// recoverPanics turns a panic in a handler into an internal error, logged with its stack. A unary response is
// written only after the handler returns, so nothing has been sent yet.
func recoverPanics(logger *slog.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (resp connect.AnyResponse, err error) {
			defer func() {
				if v := recover(); v != nil {
					logger.ErrorContext(ctx, "panic", "procedure", req.Spec().Procedure, "panic", v,
						"stack", string(debug.Stack()))
					resp, err = nil, errInternal()
				}
			}()
			return next(ctx, req)
		}
	}
}

// ErrInsufficientRole refuses a signed-in user whose role may not call a method (REST's requireRole uses it too).
var ErrInsufficientRole = apperr.New(apperr.Forbidden, "insufficient_role", "the signed-in user's role may not do this")

// requireAdmin lets only users with adminRole call Admin services. The role is the one read from the database for
// this request (internal/httpapi authenticate). A refusal is logged as authz.denied.
func requireAdmin(logger *slog.Logger, adminRole string, classes map[string]Class) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			procedure := req.Spec().Procedure
			if classes[procedure] != Admin {
				return next(ctx, req)
			}
			u, ok := authctx.From(ctx)
			if !ok {
				return nil, auth.ErrSessionInvalid
			}
			if u.Role != adminRole {
				logger.WarnContext(ctx, "operation refused", "event", "authz.denied", "user_id", u.ID,
					"procedure", procedure)
				return nil, ErrInsufficientRole
			}
			return next(ctx, req)
		}
	}
}
