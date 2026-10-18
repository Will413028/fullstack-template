package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Will413028/fullstack-template/backend/internal/httpapi/gen"
)

func TestParamBindErrorIsValidation(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{&gen.InvalidParamFormatError{ParamName: "limit", Err: errors.New(`strconv.ParseInt: parsing "0x10": invalid syntax`)}, "invalid_type"},
		{&gen.RequiredParamError{ParamName: "limit"}, "required"},
		{&gen.TooManyValuesForParamError{ParamName: "limit", Count: 2}, "invalid"},
	} {
		rec := httptest.NewRecorder()
		writeParamError(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/x", nil), discard, tc.err)
		p := assertProblem(t, rec, http.StatusUnprocessableEntity, "/problems/validation")
		if len(p.Errors) != 1 || p.Errors[0].Pointer != "/limit" || string(p.Errors[0].Code) != tc.code {
			t.Errorf("%v: errors = %+v, want /limit %s", tc.err, p.Errors, tc.code)
		}
		if strings.Contains(rec.Body.String(), "0x10") || strings.Contains(rec.Body.String(), "strconv") {
			t.Errorf("body echoes the parser error: %s", rec.Body)
		}
	}
}

func TestBodyDecodeError(t *testing.T) {
	var target struct {
		Parts struct {
			Size int `json:"max_size"`
		} `json:"parts"`
	}
	typeErr := json.Unmarshal([]byte(`{"parts":{"max_size":5.5}}`), &target)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/x", nil)
	writeBodyDecodeError(rec, req, discard, errors.Join(errors.New("can't decode JSON body"), typeErr))
	p := assertProblem(t, rec, http.StatusUnprocessableEntity, "/problems/validation")
	if len(p.Errors) != 1 || p.Errors[0].Pointer != "/parts/max_size" || p.Errors[0].Code != "invalid_type" {
		t.Errorf("errors = %+v, want /parts/max_size invalid_type", p.Errors)
	}
	if strings.Contains(rec.Body.String(), "Go struct") {
		t.Errorf("body echoes the decoder error: %s", rec.Body)
	}

	syntaxErr := json.Unmarshal([]byte(`{"a":`), &target)
	rec = httptest.NewRecorder()
	writeBodyDecodeError(rec, req, discard, syntaxErr)
	assertProblem(t, rec, http.StatusBadRequest, "/problems/bad-request")
}
