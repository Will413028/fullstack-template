package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Will413028/fullstack-template/backend/internal/httpapi/gen"
	"github.com/Will413028/fullstack-template/backend/internal/modules/auth"
)

// Clients translate a problem by its type and codes, and the frontend's type check makes its messages cover the
// contract's enums (plan D18). So every value the server sends must be in those enums: checkProblems checks each
// problem the handler tests receive, and TestProblemValuesInContract covers the values no test triggers.

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
					t.Errorf("%s %s sent %s, which the contract's enums do not list", r.Method, r.URL.Path, bad)
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
	if !gen.ProblemType(p.Type).Valid() {
		bad = append(bad, "type "+p.Type)
	}
	if p.Code != nil && !gen.ProblemCode(*p.Code).Valid() {
		bad = append(bad, "code "+*p.Code)
	}
	for _, e := range p.Errors {
		if !gen.FieldErrorCode(e.Code).Valid() {
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
		if !gen.ProblemType(p.uri()).Valid() {
			t.Errorf("problem type %s is not in the contract", p.uri())
		}
	}
	for _, p := range kindProblems {
		if !gen.ProblemType(p.uri()).Valid() {
			t.Errorf("problem type %s is not in the contract", p.uri())
		}
	}

	codes := []string{auth.CodeAccountTaken, auth.CodeInvalidCredentials, auth.CodeLoginThrottled,
		auth.ErrSessionInvalid.Code, errInsufficientRole.Code, codeCrossOrigin}
	for _, c := range codes {
		if !gen.ProblemCode(c).Valid() {
			t.Errorf("problem code %s is not in the contract", c)
		}
	}

	fieldCodes := []string{auth.CodePasswordTooCommon, "invalid", "invalid_type", "required"}
	for _, c := range schemaCodes {
		fieldCodes = append(fieldCodes, c)
	}
	for _, c := range fieldCodes {
		if !gen.FieldErrorCode(c).Valid() {
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
