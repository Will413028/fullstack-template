package httpapi

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Will413028/fullstack-template/backend/api"
	"github.com/Will413028/fullstack-template/backend/internal/modules/auth"
)

func TestAdminOperationsChecksTable(t *testing.T) {
	spec, err := loadSpec(t.Context(), api.Spec)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := adminOperations(spec, operationAccess)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"DeleteUser", "ListUsers", "PatchUser"}; !slices.Equal(admin, want) {
		t.Errorf("admin operations = %v, want %v", admin, want)
	}

	for _, tc := range []struct {
		name   string
		change func(map[string]access)
		want   string
	}{
		{"missing operation", func(m map[string]access) { delete(m, "ListUsers") },
			"operation ListUsers has no entry"},
		{"unknown entry", func(m map[string]access) { m["ListWidgets"] = accessUser },
			"names ListWidgets, which is not an operation"},
		{"public in the table only", func(m map[string]access) { m["GetUser"] = accessPublic },
			"operation GetUser: operationAccess says public true, the contract false"},
		{"public in the contract only", func(m map[string]access) { m["CreateUser"] = accessUser },
			"operation CreateUser: operationAccess says public false, the contract true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			table := maps.Clone(operationAccess)
			tc.change(table)
			if _, err := adminOperations(spec, table); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// Every operation in the contract is called the way an attacker would: without a session it is a 401 unless the
// operation is public, and with a plain user's session an admin operation is a 403.
func TestOperationAccessEnforced(t *testing.T) {
	spec, err := loadSpec(t.Context(), api.Spec)
	if err != nil {
		t.Fatal(err)
	}
	a := newAuthAPI(t)
	_, userToken := a.mustSignUp("alice")
	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			name := methodName(op.OperationID)
			url, contentType, body := anyRequest(path, method, op)
			t.Run(name, func(t *testing.T) {
				if operationAccess[name] != accessPublic {
					assertProblem(t, a.do(method, url, "", contentType, body), http.StatusUnauthorized,
						"/problems/unauthenticated")
				}
				if operationAccess[name] == accessAdmin {
					assertProblem(t, a.do(method, url, userToken, contentType, body), http.StatusForbidden,
						"/problems/forbidden")
				}
			})
		}
	}
}

var pathParam = regexp.MustCompile(`\{[^}]+\}`)

// anyRequest builds a request for op that passes the contract's checks: a fresh UUID for each path parameter, and an
// empty JSON object as the body when op takes one.
func anyRequest(path, method string, op *openapi3.Operation) (url, contentType, body string) {
	url = "/api" + pathParam.ReplaceAllString(path, uuid.Must(uuid.NewV7()).String())
	if op.RequestBody == nil {
		return url, "", ""
	}
	if method == http.MethodPatch {
		return url, "application/merge-patch+json", "{}"
	}
	return url, "application/json", "{}"
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

// Every admin operation that changes state leaves one audit row naming the admin, the action, the target and the
// request id (OWASP Logging Cheat Sheet: user administration). A new state-changing admin operation fails this test
// until it is added to send, so it cannot ship unaudited.
func TestAdminChangesAreAudited(t *testing.T) {
	spec, err := loadSpec(t.Context(), api.Spec)
	if err != nil {
		t.Fatal(err)
	}
	a := newAuthAPI(t)
	admin := a.mustAdmin()
	adminID := a.currentSession(admin).User.Id
	send := map[string]struct {
		action string
		call   func(target uuid.UUID) *httptest.ResponseRecorder
	}{
		"PatchUser": {auth.AuditUserDisabled, func(id uuid.UUID) *httptest.ResponseRecorder {
			return a.patchUser(admin, id, `{"is_disabled":true}`)
		}},
		"DeleteUser": {auth.AuditUserDeleted, func(id uuid.UUID) *httptest.ResponseRecorder {
			return a.do(http.MethodDelete, "/api/users/"+id.String(), admin, "", "")
		}},
	}
	for _, name := range stateChangingAdminOperations(spec) {
		if _, ok := send[name]; !ok {
			t.Errorf("admin operation %s changes state but has no case here", name)
		}
	}
	for name, c := range send {
		t.Run(name, func(t *testing.T) {
			u, _ := a.mustSignUp("target-" + strings.ToLower(name))
			before := len(a.auditEvents())
			if rec := c.call(u.Id); rec.Code >= 300 {
				t.Fatalf("%s = %d %s", name, rec.Code, rec.Body)
			}
			events := a.auditEvents()
			if len(events) != before+1 {
				t.Fatalf("audit rows %d -> %d, want one more", before, len(events))
			}
			e := events[len(events)-1]
			if e.actor == nil || *e.actor != adminID || e.action != c.action || e.target != u.Id || e.requestID == nil {
				t.Errorf("audit row = %+v, want actor %s, action %s, target %s and a request id", e, adminID, c.action, u.Id)
			}
		})
	}
}

// stateChangingAdminOperations lists the admin operations that are not GETs.
func stateChangingAdminOperations(spec *openapi3.T) []string {
	var names []string
	for _, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			if name := methodName(op.OperationID); method != http.MethodGet && operationAccess[name] == accessAdmin {
				names = append(names, name)
			}
		}
	}
	return names
}
