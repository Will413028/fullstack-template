package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Will413028/fullstack-template/backend/api"
	"github.com/Will413028/fullstack-template/backend/internal/httpapi/gen"
)

// newStubAPI assembles the full handler chain around testdata/test-openapi.yaml with stub operations: POST
// /api/widgets echoes the body it receives with 201, GET /api/fail panics, other operations answer 200.
func newStubAPI(t *testing.T, deps Deps) http.Handler {
	t.Helper()
	data, err := os.ReadFile("testdata/test-openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	spec, err := loadSpec(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	routes := http.NewServeMux()
	routes.HandleFunc("POST /api/widgets", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	})
	routes.HandleFunc("GET /api/widgets", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	routes.HandleFunc("GET /api/widgets/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	routes.HandleFunc("GET /api/fail", func(http.ResponseWriter, *http.Request) { panic("boom") })
	if deps.Logger == nil {
		deps.Logger = discard
	}
	if deps.Ready == nil {
		deps.Ready = ok
	}
	return assemble(deps, spec, routes)
}

func do(t *testing.T, h http.Handler, method, target, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, "http://app.test"+target, strings.NewReader(body))
	if body == "" {
		req = httptest.NewRequestWithContext(t.Context(), method, "http://app.test"+target, http.NoBody)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func postWidget(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, h, http.MethodPost, "/api/widgets", "application/json", body)
}

func TestValidRequestReachesHandler(t *testing.T) {
	h := newStubAPI(t, Deps{})
	body := `{"name":"abc","size":2}`
	rec := postWidget(t, h, body)
	if rec.Code != http.StatusCreated || rec.Body.String() != body {
		t.Fatalf("code = %d body = %q, want 201 with the body passed through", rec.Code, rec.Body)
	}
	for _, target := range []string{"/api/widgets?limit=5", "/api/widgets/019a0f3e-7c4b-7d2a-9b1e-3f5a6c7d8e9f"} {
		if rec := do(t, h, http.MethodGet, target, "", ""); rec.Code != http.StatusOK {
			t.Fatalf("GET %s: code = %d (body %q)", target, rec.Code, rec.Body)
		}
	}
}

func TestValidationMinLength(t *testing.T) {
	p := assertProblem(t, postWidget(t, newStubAPI(t, Deps{}), `{"name":"ab"}`), http.StatusUnprocessableEntity,
		"/problems/validation")
	if len(p.Errors) == 0 {
		t.Fatal("no errors")
	}
	if e := p.Errors[0]; e.Pointer != "/name" || e.Code != "too_short" || e.Detail == "" {
		t.Errorf("errors[0] = %+v, want pointer /name and code too_short", e)
	}
}

func TestValidationCodes(t *testing.T) {
	h := newStubAPI(t, Deps{})
	for _, tc := range []struct {
		name, method, target, body string
		want                       gen.FieldError
	}{
		{"required", "POST", "/api/widgets", `{}`, gen.FieldError{Pointer: "/name", Code: "required"}},
		{"too_short", "POST", "/api/widgets", `{"name":"ab"}`, gen.FieldError{Pointer: "/name", Code: "too_short"}},
		{"too_long", "POST", "/api/widgets", `{"name":"abcdefghijk"}`, gen.FieldError{Pointer: "/name", Code: "too_long"}},
		{"too_long items", "POST", "/api/widgets", `{"name":"abc","tags":["a","b","c"]}`, gen.FieldError{Pointer: "/tags", Code: "too_long"}},
		{"pattern", "POST", "/api/widgets", `{"name":"abc","slug":"ABC"}`, gen.FieldError{Pointer: "/slug", Code: "pattern"}},
		{"enum", "POST", "/api/widgets", `{"name":"abc","color":"green"}`, gen.FieldError{Pointer: "/color", Code: "enum"}},
		{"format", "POST", "/api/widgets", `{"name":"abc","ref":"not-a-uuid"}`, gen.FieldError{Pointer: "/ref", Code: "format"}},
		{"out_of_range", "POST", "/api/widgets", `{"name":"abc","size":9}`, gen.FieldError{Pointer: "/size", Code: "out_of_range"}},
		{"invalid_type", "POST", "/api/widgets", `{"name":"abc","size":"big"}`, gen.FieldError{Pointer: "/size", Code: "invalid_type"}},
		{"null_not_allowed", "POST", "/api/widgets", `{"name":"abc","note":null}`, gen.FieldError{Pointer: "/note", Code: "null_not_allowed"}},
		{"additional_property", "POST", "/api/widgets", `{"name":"abc","extra":1}`, gen.FieldError{Pointer: "/extra", Code: "additional_property"}},
		{"nested pointer is escaped", "POST", "/api/widgets", `{"name":"abc","parts":{"a/b":"xy"}}`, gen.FieldError{Pointer: "/parts/a~1b", Code: "too_long"}},
		{"unlisted keyword falls back", "POST", "/api/widgets", `{"name":"abc","count":3}`, gen.FieldError{Pointer: "/count", Code: "invalid"}},
		{"missing body", "POST", "/api/widgets", "", gen.FieldError{Pointer: "", Code: "required"}},
		{"query out_of_range", "GET", "/api/widgets?limit=0", "", gen.FieldError{Pointer: "/limit", Code: "out_of_range"}},
		{"query invalid_type", "GET", "/api/widgets?limit=abc", "", gen.FieldError{Pointer: "/limit", Code: "invalid_type"}},
		{"query empty value", "GET", "/api/widgets?limit=", "", gen.FieldError{Pointer: "/limit", Code: "invalid"}},
		{"path format", "GET", "/api/widgets/not-a-uuid", "", gen.FieldError{Pointer: "/id", Code: "format"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ct := ""
			if tc.method == http.MethodPost {
				ct = "application/json"
			}
			p := assertProblem(t, do(t, h, tc.method, tc.target, ct, tc.body), http.StatusUnprocessableEntity,
				"/problems/validation")
			if len(p.Errors) != 1 {
				t.Fatalf("errors = %+v, want one", p.Errors)
			}
			if got := p.Errors[0]; got.Pointer != tc.want.Pointer || got.Code != tc.want.Code {
				t.Errorf("errors[0] = %+v, want pointer %q code %q", got, tc.want.Pointer, tc.want.Code)
			}
		})
	}
}

func TestValidationListsEveryError(t *testing.T) {
	p := assertProblem(t, postWidget(t, newStubAPI(t, Deps{}), `{"name":"ab","size":9,"x":1,"y":2}`),
		http.StatusUnprocessableEntity, "/problems/validation")
	got := map[string]string{}
	for _, e := range p.Errors {
		got[e.Pointer] = e.Code
	}
	want := map[string]string{"/name": "too_short", "/size": "out_of_range", "/x": "additional_property", "/y": "additional_property"}
	if len(got) != len(want) || len(p.Errors) != len(want) {
		t.Fatalf("errors = %+v, want %v", p.Errors, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: code %q, want %q", k, got[k], v)
		}
	}
}

func TestUnsupportedMediaType(t *testing.T) {
	h := newStubAPI(t, Deps{})
	for _, ct := range []string{"text/x-canary", ""} {
		rec := do(t, h, http.MethodPost, "/api/widgets", ct, `{"name":"abc"}`)
		p := assertProblem(t, rec, http.StatusUnsupportedMediaType, "/problems/unsupported-media-type")
		if strings.Contains(rec.Body.String(), "canary") || p.Detail == nil || !strings.Contains(*p.Detail, "application/json") {
			t.Errorf("detail = %v, want the accepted types without echoing the request header", p.Detail)
		}
	}
	// Media type parameters are fine.
	if rec := do(t, h, http.MethodPost, "/api/widgets", "application/json; charset=utf-8", `{"name":"abc"}`); rec.Code != http.StatusCreated {
		t.Errorf("application/json with charset: code = %d, want 201", rec.Code)
	}
}

func TestMalformedJSON(t *testing.T) {
	h := newStubAPI(t, Deps{})
	for _, body := range []string{`{"name":`, `{"name":"abc"} trailing`, `{"name":"abc"}{}`} {
		assertProblem(t, postWidget(t, h, body), http.StatusBadRequest, "/problems/bad-request")
	}
}

// failingBody fails like a body whose client went away mid-request.
type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestBodyReadErrorIsNotValidation(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://app.test/api/widgets", failingBody{})
	req.ContentLength = 10
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newStubAPI(t, Deps{}).ServeHTTP(rec, req)
	assertProblem(t, rec, http.StatusInternalServerError, "/problems/internal")
}

func TestUndefinedRoute(t *testing.T) {
	h := newStubAPI(t, Deps{})
	assertProblem(t, do(t, h, http.MethodGet, "/api/nope", "", ""), http.StatusNotFound, "/problems/not-found")
	assertProblem(t, do(t, h, http.MethodDelete, "/api/widgets", "", ""), http.StatusMethodNotAllowed,
		"/problems/method-not-allowed")
	// The contract's paths have no /api prefix; only /api/widgets exists, not /widgets.
	assertProblem(t, do(t, h, http.MethodGet, "/widgets", "", ""), http.StatusNotFound, "/problems/not-found")
}

func TestValidatedBodyTooLarge(t *testing.T) {
	big := `{"name":"` + strings.Repeat("a", maxBodyBytes) + `"}`
	assertProblem(t, postWidget(t, newStubAPI(t, Deps{}), big), http.StatusRequestEntityTooLarge,
		"/problems/payload-too-large")
}

func TestPanicInOperationReturnsProblem(t *testing.T) {
	assertProblem(t, do(t, newStubAPI(t, Deps{}), http.MethodGet, "/api/fail", "", ""),
		http.StatusInternalServerError, "/problems/internal")
}

func TestHealthOutsideValidation(t *testing.T) {
	h := newStubAPI(t, Deps{Docs: true})
	assertStatusBody(t, get(t, h, "/health"), http.StatusOK, "ok")
	assertStatusBody(t, get(t, h, "/health/ready"), http.StatusOK, "ready")
}

func TestDocs(t *testing.T) {
	dev := newTestHandlerDeps(t, Deps{Ready: ok, Logger: discard, Docs: true})
	rec := get(t, dev, "/api/openapi.yaml")
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), api.Spec) {
		t.Fatalf("dev /api/openapi.yaml: code = %d, want 200 with the embedded spec", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type = %q", ct)
	}
	rec = get(t, dev, "/api/docs")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `integrity="sha384-`) {
		t.Fatalf("dev /api/docs: code = %d, want 200 with SRI", rec.Code)
	}

	prod := newTestHandlerDeps(t, Deps{Ready: ok, Logger: discard})
	for _, path := range []string{"/api/openapi.yaml", "/api/docs"} {
		assertProblem(t, get(t, prod, path), http.StatusNotFound, "/problems/not-found")
	}
}

func newTestHandlerDeps(t *testing.T, deps Deps) http.Handler {
	t.Helper()
	h, err := NewHandler(t.Context(), deps)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
