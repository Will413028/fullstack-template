package httpapi

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveError(t *testing.T, err error, logger *slog.Logger) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rec.Header().Set(headerRequestID, "test") // withRequestInfo's, which this test leaves out
	writeError(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/x", nil), logger, err)
	return rec
}

func TestUnhandledErrorHidden(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	internal := `pq: relation "users" does not exist at db.internal:5432 (canary-7f3a)`
	rec := serveError(t, errors.New(internal), logger)
	decodeWireError(t, rec, http.StatusInternalServerError, "internal")
	if strings.Contains(rec.Body.String(), "canary-7f3a") {
		t.Errorf("500 body leaks the error: %s", rec.Body)
	}
	if !strings.Contains(logs.String(), "canary-7f3a") {
		t.Errorf("the error is not logged: %s", logs.String())
	}
}

func TestPanicReturnsInternal(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	h := recoverPanics(logger, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	rec.Header().Set(headerRequestID, "test") // withRequestInfo's, which this test leaves out
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/x", nil))
	decodeWireError(t, rec, http.StatusInternalServerError, "internal")
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
		if strings.Contains(rec.Body.String(), `"code"`) {
			t.Errorf("an error was appended to the started response: %s", rec.Body)
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
