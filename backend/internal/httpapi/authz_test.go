package httpapi

import (
	"maps"
	"net/http"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	adminusersv1connect "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/admin/users/v1/usersv1connect"
	usersv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/users/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc"
	"github.com/Will413028/template-go-nextjs/backend/internal/modules/auth"
)

// Authorization follows the service (rpc.ClassOf): Public services need no session, App services a signed-in user,
// Admin services an admin. The tests below read the procedures from connectServices, so a new service or method is
// covered without a change here.

// mountedProcedures are the procedures connectServices mounts in production, with their class.
func mountedProcedures(t *testing.T) map[string]rpc.Class {
	t.Helper()
	out := map[string]rpc.Class{}
	for _, s := range connectServices(Deps{}) {
		class, ok := rpc.ClassOf(s.Desc.FullName())
		if !ok {
			t.Fatalf("service %s has no class", s.Desc.FullName())
		}
		methods := s.Desc.Methods()
		for i := range methods.Len() {
			out[rpc.Procedure(methods.Get(i))] = class
		}
	}
	if len(out) == 0 {
		t.Fatal("connectServices mounts nothing")
	}
	return out
}

// Every mounted procedure is called the way an attacker would: without a session an App or Admin procedure is
// unauthenticated and a Public one reaches its handler; with a plain user's session an Admin procedure is
// permission_denied (INSUFFICIENT_ROLE) before its handler runs.
func TestProcedureAccessEnforced(t *testing.T) {
	a := newAuthAPI(t)
	_, user := a.mustSignUp("alice")
	asUser := map[string]string{"Cookie": cookieName + "=" + user}
	for procedure, class := range mountedProcedures(t) {
		t.Run(procedure, func(t *testing.T) { assertAccess(t, a.h, procedure, class, asUser) })
	}
}

// assertAccess calls procedure anonymously and, for an Admin procedure, with the headers of a plain user.
func assertAccess(t *testing.T, h http.Handler, procedure string, class rpc.Class, asUser map[string]string) {
	t.Helper()
	rec := connectPost(t, h, procedure, `{}`, nil)
	if class == rpc.Public {
		switch rec.Code {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError:
			t.Errorf("anonymous = %d %s, want the handler's answer", rec.Code, rec.Body)
		}
		return
	}
	if r := decodeWireError(t, rec, http.StatusUnauthorized, "unauthenticated").reason(t); r != "SESSION_INVALID" {
		t.Errorf("anonymous: reason = %q, want SESSION_INVALID", r)
	}
	if class != rpc.Admin {
		return
	}
	rec = connectPost(t, h, procedure, `{}`, asUser)
	if r := decodeWireError(t, rec, http.StatusForbidden, "permission_denied").reason(t); r != "INSUFFICIENT_ROLE" {
		t.Errorf("as a user: reason = %q, want INSUFFICIENT_ROLE", r)
	}
}

// Every role an account can have, the ones users.role's CHECK allows, is a role the auth module defines and is sent as
// a proto Role other than ROLE_UNSPECIFIED, each as its own value. No database: the CHECK is read from schema.sql.
func TestRolesMapped(t *testing.T) {
	schema, err := os.ReadFile("../modules/auth/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	check := regexp.MustCompile(`\brole\s+text\b[^\n]*CHECK \(role IN \(([^)]*)\)\)`).FindSubmatch(schema)
	if check == nil {
		t.Fatal("users.role's CHECK (role IN (...)) not found in the auth module's schema.sql")
	}
	var allowed []string
	for _, v := range strings.Split(string(check[1]), ",") {
		allowed = append(allowed, strings.Trim(strings.TrimSpace(v), "'"))
	}
	slices.Sort(allowed)
	if want := []string{auth.RoleAdmin, auth.RoleUser}; !slices.Equal(allowed, want) {
		t.Errorf("users.role allows %v, the auth module defines %v", allowed, want)
	}
	sent := map[usersv1.Role]string{}
	for _, role := range allowed {
		got := toUser(auth.User{Role: role}).GetRole()
		if got == usersv1.Role_ROLE_UNSPECIFIED {
			t.Errorf("role %q is sent as %s", role, got)
		}
		if other, dup := sent[got]; dup {
			t.Errorf("roles %q and %q are both sent as %s", other, role, got)
		}
		sent[got] = role
	}
}

type auditRow struct {
	actor     *uuid.UUID
	action    string
	target    uuid.UUID
	requestID *string
}

func (a *authAPI) auditEvents() []auditRow {
	a.t.Helper()
	r, err := a.pool.Query(a.t.Context(), `SELECT actor_id, action, target_id, request_id FROM audit_events ORDER BY id`)
	if err != nil {
		a.t.Fatal(err)
	}
	rows, err := pgx.CollectRows(r, func(row pgx.CollectableRow) (auditRow, error) {
		var e auditRow
		return e, row.Scan(&e.actor, &e.action, &e.target, &e.requestID)
	})
	if err != nil {
		a.t.Fatal(err)
	}
	return rows
}

// stateChangingAdminProcedures are the mounted Admin procedures that may change state: every one not marked
// NO_SIDE_EFFECTS.
func stateChangingAdminProcedures(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, s := range connectServices(Deps{}) {
		if class, _ := rpc.ClassOf(s.Desc.FullName()); class != rpc.Admin {
			continue
		}
		methods := s.Desc.Methods()
		for i := range methods.Len() {
			if md := methods.Get(i); !rpc.SideEffectFree(md) {
				out = append(out, rpc.Procedure(md))
			}
		}
	}
	slices.Sort(out)
	return out
}

// Every Admin procedure that changes state leaves one audit row naming the admin, the action, the target and the
// request id (OWASP Logging Cheat Sheet: user administration). The procedures come from the mounted services, so a new
// state-changing Admin procedure fails this test until it has a case here, and cannot ship unaudited
// (backend/AGENTS.md rule 11).
func TestAdminChangesAreAudited(t *testing.T) {
	a := newAuthAPI(t)
	admin := a.mustAdmin()
	adminID := uuid.MustParse(a.currentSession(admin).GetUser().GetId())
	send := map[string]struct {
		action string
		call   func(target string) error
	}{
		adminusersv1connect.UserAdminServiceUpdateUserProcedure: {auth.AuditUserDisabled, func(id string) error {
			_, err := a.disable(admin, id, true)
			return err
		}},
		adminusersv1connect.UserAdminServiceDeleteUserProcedure: {auth.AuditUserDeleted, func(id string) error {
			return a.deleteUser(admin, id)
		}},
		adminusersv1connect.UserAdminServiceResetSignInLockProcedure: {auth.AuditUserUnlocked, func(id string) error {
			return a.resetSignInLock(admin, id)
		}},
	}
	cases := slices.Sorted(maps.Keys(send))
	if procedures := stateChangingAdminProcedures(t); !slices.Equal(cases, procedures) {
		t.Errorf("cases %v, state-changing Admin procedures %v: each needs a case, and each case a procedure", cases,
			procedures)
	}
	for procedure, c := range send {
		t.Run(procedure, func(t *testing.T) {
			u, _ := a.mustSignUp("target-" + strings.ToLower(path.Base(procedure)))
			before := len(a.auditEvents())
			if err := c.call(u.GetId()); err != nil {
				t.Fatalf("%s: %v", procedure, err)
			}
			events := a.auditEvents()
			if len(events) != before+1 {
				t.Fatalf("audit rows %d -> %d, want one more", before, len(events))
			}
			e := events[len(events)-1]
			if e.actor == nil || *e.actor != adminID || e.action != c.action || e.target.String() != u.GetId() || e.requestID == nil {
				t.Errorf("audit row = %+v, want actor %s, action %s, target %s and a request id", e, adminID, c.action, u.GetId())
			}
		})
	}
}
