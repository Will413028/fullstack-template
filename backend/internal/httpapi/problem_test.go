package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Will413028/fullstack-template/backend/internal/httpapi/gen"
	"github.com/Will413028/fullstack-template/backend/internal/platform/apperr"
)

// assertProblem checks rec is an RFC 9457 problem with the given status and type, and returns it.
func assertProblem(t *testing.T, rec *httptest.ResponseRecorder, status int, typ string) gen.ValidationProblem {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("code = %d, want %d (body %q)", rec.Code, status, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != problemContentType {
		t.Errorf("Content-Type = %q, want %s", ct, problemContentType)
	}
	var p gen.ValidationProblem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("body is not a problem: %v (%q)", err, rec.Body)
	}
	if string(p.Type) != typ || p.Status != status || p.Title == "" {
		t.Errorf("problem = %+v, want type %s and status %d with a title", p, typ, status)
	}
	return p
}

func serveError(t *testing.T, err error, logger *slog.Logger) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	writeError(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/x", nil), logger, err)
	return rec
}

func TestAppErrToProblem(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		typ    string
	}{
		{apperr.New(apperr.NotFound, "item_not_found", "no such item"), 404, "/problems/not-found"},
		{apperr.New(apperr.Conflict, "account_taken", "account exists"), 409, "/problems/conflict"},
		{apperr.New(apperr.Unauthenticated, "session_invalid", ""), 401, "/problems/unauthenticated"},
		{apperr.New(apperr.Forbidden, "admin_required", ""), 403, "/problems/forbidden"},
		{apperr.New(apperr.RateLimited, "too_many_requests", ""), 429, "/problems/rate-limited"},
		// Wrapped errors keep their kind.
		{fmt.Errorf("create: %w", apperr.New(apperr.Conflict, "account_taken", "")), 409, "/problems/conflict"},
	} {
		p := assertProblem(t, serveError(t, tc.err, discard), tc.status, tc.typ)
		want, _ := apperr.As(tc.err)
		if p.Code == nil || string(*p.Code) != want.Code {
			t.Errorf("%v: code = %v, want %s", tc.err, p.Code, want.Code)
		}
	}
}

func TestAppErrValidation(t *testing.T) {
	rec := serveError(t, apperr.Invalid("/password", "password_too_common", "on the common-password list"), discard)
	p := assertProblem(t, rec, http.StatusUnprocessableEntity, "/problems/validation")
	if len(p.Errors) != 1 || p.Errors[0].Pointer != "/password" || p.Errors[0].Code != "password_too_common" {
		t.Errorf("errors = %+v", p.Errors)
	}
}

func TestRateLimitedRetryAfter(t *testing.T) {
	err := &apperr.Error{Kind: apperr.RateLimited, Code: "too_many_requests", RetryAfter: 1500 * time.Millisecond}
	rec := serveError(t, err, discard)
	assertProblem(t, rec, http.StatusTooManyRequests, "/problems/rate-limited")
	if got := rec.Header().Get("Retry-After"); got != "2" {
		t.Errorf("Retry-After = %q, want 2 (rounded up)", got)
	}
}

func TestUnhandledErrorHidden(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	internal := `pq: relation "users" does not exist at db.internal:5432 (canary-7f3a)`
	rec := serveError(t, errors.New(internal), logger)
	p := assertProblem(t, rec, http.StatusInternalServerError, "/problems/internal")
	if strings.Contains(rec.Body.String(), "canary-7f3a") || p.Detail != nil {
		t.Errorf("500 body leaks the error: %s", rec.Body)
	}
	if !strings.Contains(logs.String(), "canary-7f3a") {
		t.Errorf("the error is not logged: %s", logs.String())
	}
}

func TestPanicReturnsProblem(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	h := recoverPanics(logger, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/x", nil))
	assertProblem(t, rec, http.StatusInternalServerError, "/problems/internal")
	if strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("500 body leaks the panic value: %s", rec.Body)
	}
	if !strings.Contains(logs.String(), `"panic":"boom"`) || !strings.Contains(logs.String(), "goroutine") {
		t.Errorf("log lacks the panic value or stack: %s", logs.String())
	}
}

func TestPanicAbortHandlerPropagates(t *testing.T) {
	h := recoverPanics(discard, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if v := recover(); v != http.ErrAbortHandler { //nolint:errorlint // the exact sentinel value
			t.Errorf("recovered %v, want http.ErrAbortHandler re-raised", v)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
}

func TestPanicAfterResponseStartedAborts(t *testing.T) {
	h := recoverPanics(discard, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[`))
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	defer func() {
		if v := recover(); v != http.ErrAbortHandler { //nolint:errorlint // the exact sentinel value
			t.Errorf("recovered %v, want http.ErrAbortHandler", v)
		}
		if strings.Contains(rec.Body.String(), "problems") {
			t.Errorf("a problem was appended to the started response: %s", rec.Body)
		}
	}()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/x", nil))
}

func TestErrorAfterResponseStartedAborts(t *testing.T) {
	h := recoverPanics(discard, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		writeError(w, r, discard, errors.New("encode failed"))
	}))
	rec := httptest.NewRecorder()
	defer func() {
		if v := recover(); v != http.ErrAbortHandler { //nolint:errorlint // the exact sentinel value
			t.Errorf("recovered %v, want http.ErrAbortHandler", v)
		}
	}()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/x", nil))
}
