package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/Will413028/fullstack-template/backend/api"
	"github.com/Will413028/fullstack-template/backend/internal/modules/auth"
)

// Clients translate a problem by its type and codes, and the frontend's type check makes its messages cover the
// values the contract lists in x-extensible-enum (docs/adr/2026-10-18-api-and-auth-baseline.md D3). So every value
// the server sends must be listed: checkProblems checks each problem the handler tests receive, and
// TestProblemValuesInContract covers the values no test triggers.

// contractValues is the x-extensible-enum list of each problem property, from the embedded contract.
var contractValues = sync.OnceValues(func() (map[string]map[string]bool, error) {
	spec, err := loadSpec(context.Background(), api.Spec)
	if err != nil {
		return nil, err
	}
	values := map[string]map[string]bool{}
	for key, prop := range map[string][2]string{
		"type": {"Problem", "type"}, "code": {"Problem", "code"}, "field": {"FieldError", "code"},
	} {
		schema := spec.Components.Schemas[prop[0]].Value.Properties[prop[1]].Value
		list, ok := schema.Extensions["x-extensible-enum"].([]any)
		if !ok {
			return nil, fmt.Errorf("%s.%s has no x-extensible-enum list", prop[0], prop[1])
		}
		values[key] = map[string]bool{}
		for _, v := range list {
			values[key][fmt.Sprint(v)] = true
		}
	}
	return values, nil
})

// inContract reports whether the contract lists value for the problem property key (type, code or field).
func inContract(key, value string) bool {
	values, err := contractValues()
	if err != nil {
		panic(err)
	}
	return values[key][value]
}

// problemTee passes a response through and keeps a copy of its body.
type problemTee struct {
	http.ResponseWriter
	body bytes.Buffer
}

func (p *problemTee) Write(b []byte) (int, error) {
	p.body.Write(b)
	return p.ResponseWriter.Write(b)
}

func (p *problemTee) Unwrap() http.ResponseWriter { return p.ResponseWriter }

// checkProblems wraps h so that every problem it sends is checked against the contract. Errorf, not Fatalf: handlers
// run in goroutines too.
func checkProblems(t *testing.T, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tee := &problemTee{ResponseWriter: w}
		defer func() {
			if tee.Header().Get("Content-Type") == problemContentType {
				for _, bad := range valuesOutsideContract(tee.body.Bytes()) {
					t.Errorf("%s %s sent %s, which the contract does not list", r.Method, r.URL.Path, bad)
				}
			}
		}()
		h.ServeHTTP(tee, r)
	})
}

// valuesOutsideContract returns the type and codes in a problem body that are not in the contract's enums.
func valuesOutsideContract(body []byte) []string {
	var p struct {
		Type   string  `json:"type"`
		Code   *string `json:"code"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return []string{"a body that is not JSON"}
	}
	var bad []string
	if !inContract("type", p.Type) {
		bad = append(bad, "type "+p.Type)
	}
	if p.Code != nil && !inContract("code", *p.Code) {
		bad = append(bad, "code "+*p.Code)
	}
	for _, e := range p.Errors {
		if !inContract("field", e.Code) {
			bad = append(bad, "field code "+e.Code)
		}
	}
	return bad
}

func TestProblemValuesInContract(t *testing.T) {
	types := []problemType{
		problemBadRequest, problemUnauthenticated, problemForbidden, problemNotFound, problemMethodNotAllowed,
		problemConflict, problemPayloadTooLarge, problemUnsupportedMediaType, problemValidation, problemRateLimited,
		problemInternal, problemUnavailable,
	}
	for _, p := range types {
		if !inContract("type", p.uri()) {
			t.Errorf("problem type %s is not in the contract", p.uri())
		}
	}
	for _, p := range kindProblems {
		if !inContract("type", p.uri()) {
			t.Errorf("problem type %s is not in the contract", p.uri())
		}
	}

	codes := []string{auth.CodeAccountTaken, auth.CodeInvalidCredentials, auth.CodeLoginThrottled,
		auth.ErrSessionInvalid.Code, errInsufficientRole.Code, codeCrossOrigin}
	for _, c := range codes {
		if !inContract("code", c) {
			t.Errorf("problem code %s is not in the contract", c)
		}
	}

	fieldCodes := []string{auth.CodePasswordTooCommon, "invalid", "invalid_type", "required"}
	for _, c := range schemaCodes {
		fieldCodes = append(fieldCodes, c)
	}
	for _, c := range fieldCodes {
		if !inContract("field", c) {
			t.Errorf("field error code %s is not in the contract", c)
		}
	}
}

// the check itself: a value outside the enums is reported
func TestValuesOutsideContract(t *testing.T) {
	body := `{"type":"/problems/teapot","code":"brewing","errors":[{"pointer":"/a","code":"too_hot","detail":""}]}`
	got := valuesOutsideContract([]byte(body))
	want := []string{"type /problems/teapot", "code brewing", "field code too_hot"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
	if bad := valuesOutsideContract([]byte(`{"type":"/problems/validation","errors":[{"code":"too_short"}]}`)); bad != nil {
		t.Errorf("contract values reported: %v", bad)
	}
}
