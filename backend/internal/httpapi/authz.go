package httpapi

import (
	"context"
	"net/http"
	"slices"

	"github.com/Will413028/fullstack-template/backend/internal/httpapi/gen"
	"github.com/Will413028/fullstack-template/backend/internal/modules/auth"
	"github.com/Will413028/fullstack-template/backend/internal/platform/apperr"
	"github.com/Will413028/fullstack-template/backend/internal/platform/authctx"
)

// Authorization (docs/adr/2026-10-18-api-and-auth-baseline.md D7): resources a user owns are scoped in their queries
// (owner_id = the current user, 404 for anyone else); operations reserved to a role are listed here and rejected with
// 403 before the handler runs.

var errInsufficientRole = apperr.New(apperr.Forbidden, "insufficient_role", "the signed-in user's role may not do this")

// requireRole lets only users with role call the named operations (generated method names, see operations). The
// role is the one read from the database for this request (authenticate).
func requireRole(role string, names ...string) gen.StrictMiddlewareFunc {
	return func(f gen.StrictHandlerFunc, name string) gen.StrictHandlerFunc {
		if !slices.Contains(names, name) {
			return f
		}
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
			u, ok := authctx.From(ctx)
			if !ok {
				return nil, auth.ErrSessionInvalid
			}
			if u.Role != role {
				return nil, errInsufficientRole
			}
			return f(ctx, w, r, request)
		}
	}
}
