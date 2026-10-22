package httpapi

import (
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"

	"github.com/Will413028/fullstack-template/backend/api"
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
