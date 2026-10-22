package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Will413028/fullstack-template/backend/internal/httpapi/gen"
	"github.com/Will413028/fullstack-template/backend/internal/modules/auth"
	"github.com/Will413028/fullstack-template/backend/internal/platform/apperr"
	"github.com/Will413028/fullstack-template/backend/internal/platform/authctx"
)

// Authorization (docs/adr/2026-10-18-api-and-auth-baseline.md D7): resources a user owns are scoped in their queries
// (owner_id = the current user, 404 for anyone else); operations reserved to a role are listed here and rejected with
// 403 before the handler runs.

// access says who may call an operation.
type access int

const (
	// accessPublic needs no session: the contract gives the operation security: [].
	accessPublic access = iota + 1
	// accessUser needs any signed-in user.
	accessUser
	// accessAdmin needs a signed-in admin; requireRole rejects everyone else with 403.
	accessAdmin
)

// operationAccess decides who may call each operation (generated method names, see operations). Every operation is
// listed, or NewHandler fails: a new operation's access is a decision, not a default that leaves an admin operation
// open to every user.
var operationAccess = map[string]access{
	"CreateUser":           accessPublic,
	"CreateSession":        accessPublic,
	"DeleteCurrentSession": accessPublic,
	"GetCurrentUser":       accessUser,
	"GetUser":              accessUser,
	"DeleteCurrentUser":    accessUser,
	"ListSessions":         accessUser,
	"DeleteAllSessions":    accessUser,
	"GetCurrentSession":    accessUser,
	"GetSession":           accessUser,
	"DeleteSession":        accessUser,
	"CreateItem":           accessUser,
	"ListItems":            accessUser,
	"GetItem":              accessUser,
	"UpdateItem":           accessUser,
	"DeleteItem":           accessUser,
	"ListUsers":            accessAdmin,
	"PatchUser":            accessAdmin,
	"DeleteUser":           accessAdmin,
	"GetSignInLock":        accessAdmin,
	"DeleteSignInLock":     accessAdmin,
}

// adminOperations checks table against the generated interface and the contract, and returns the operations that
// need an admin. Every operation must be in table and every entry must be an operation; an entry is accessPublic
// exactly when the contract gives its operation security: [].
func adminOperations(spec *openapi3.T, table map[string]access) ([]string, error) {
	iface := reflect.TypeFor[gen.StrictServerInterface]()
	var errs []error
	for i := range iface.NumMethod() {
		if name := iface.Method(i).Name; table[name] == 0 {
			errs = append(errs, fmt.Errorf("operation %s has no entry in operationAccess", name))
		}
	}
	public := map[string]bool{}
	for _, item := range spec.Paths.Map() {
		for _, op := range item.Operations() {
			public[methodName(op.OperationID)] = op.Security != nil && len(*op.Security) == 0
		}
	}
	var admin []string
	for name, a := range table {
		if _, ok := iface.MethodByName(name); !ok {
			errs = append(errs, fmt.Errorf("operationAccess names %s, which is not an operation", name))
			continue
		}
		if (a == accessPublic) != public[name] {
			errs = append(errs, fmt.Errorf("operation %s: operationAccess says public %t, the contract %t",
				name, a == accessPublic, public[name]))
		}
		if a == accessAdmin {
			admin = append(admin, name)
		}
	}
	slices.Sort(admin)
	return admin, errors.Join(errs...)
}

// methodName is the generated method name for an operationId (createUser -> CreateUser).
func methodName(operationID string) string {
	r := []rune(operationID)
	if len(r) == 0 {
		return ""
	}
	return string(unicode.ToUpper(r[0])) + strings.TrimPrefix(operationID, string(r[0]))
}

var errInsufficientRole = apperr.New(apperr.Forbidden, "insufficient_role", "the signed-in user's role may not do this")

// requireRole lets only users with role call the named operations (generated method names, see operations). The
// role is the one read from the database for this request (authenticate). A refusal is logged as authz.denied.
func requireRole(logger *slog.Logger, role string, names ...string) gen.StrictMiddlewareFunc {
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
				logger.WarnContext(ctx, "operation refused", "event", "authz.denied", "user_id", u.ID, "operation", name)
				return nil, errInsufficientRole
			}
			return f(ctx, w, r, request)
		}
	}
}
