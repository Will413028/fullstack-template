package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newStubAPI assembles the full handler chain around a stub in place of the Connect services: GET and POST
// /api/widgets answer 200, GET /api/fail panics, any other /api path is not_found.
func newStubAPI(t *testing.T, deps Deps) http.Handler {
	t.Helper()
	if deps.Logger == nil {
		deps.Logger = discard
	}
	if deps.Ready == nil {
		deps.Ready = ok
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	stub := http.NewServeMux()
	stub.HandleFunc("GET /api/widgets", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	stub.HandleFunc("POST /api/widgets", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	stub.HandleFunc("GET /api/fail", func(http.ResponseWriter, *http.Request) { panic("boom") })
	stub.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { writeError(w, r, deps.Logger, errNoProcedure) })
	return assemble(deps, stub)
}

// do sends a same-origin request without a body.
func do(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, "http://app.test"+target, http.NoBody)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newTestHandlerDeps(t *testing.T, deps Deps) http.Handler {
	t.Helper()
	if deps.RateLimitPerMin == 0 {
		deps.RateLimitPerMin = 5
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	h, err := NewHandler(deps)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
