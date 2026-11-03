package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	commonv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/common/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/gen/app/users/v1/usersv1connect"
	"github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc"
	testingv1 "github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc/testdata/gen/app/testing/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc/testdata/gen/app/testing/v1/testingv1connect"
	"github.com/Will413028/template-go-nextjs/backend/internal/modules/auth"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/apperr"
)

// The Connect entry through NewHandler, with the test services of internal/httpapi/rpc/testdata (one per caller
// class). Production services join connectServices step by step (docs/plans/2026-11-03-connect-contract.md).

// testThings implements the test services. UpdateXThing acts on the thing's name: "boom" fails with an unexpected
// error, "panic" panics, "taken" fails with an expected one (account_taken); any other name is echoed back.
type testThings struct {
	testingv1connect.UnimplementedTestingInternalServiceHandler
	testingv1connect.UnimplementedTestingSideEffectAppServiceHandler
	testingv1connect.UnimplementedTestingOrphanAppServiceHandler
}

func thingFor(id string) *testingv1.Thing { return &testingv1.Thing{Id: id, Name: "thing " + id} }

func update(thing *testingv1.Thing) (*testingv1.Thing, error) {
	switch thing.GetName() {
	case "boom":
		return nil, errors.New("query failed: password=hunter2")
	case "panic":
		panic("handler panicked")
	case "taken":
		return nil, apperr.New(apperr.AlreadyExists, auth.CodeAccountTaken, "account is taken")
	}
	return thing, nil
}

func (testThings) GetPublicThing(_ context.Context, req *connect.Request[testingv1.GetPublicThingRequest]) (*connect.Response[testingv1.GetPublicThingResponse], error) {
	return connect.NewResponse(&testingv1.GetPublicThingResponse{Thing: thingFor(req.Msg.GetId())}), nil
}

func (testThings) UpdatePublicThing(_ context.Context, req *connect.Request[testingv1.UpdatePublicThingRequest]) (*connect.Response[testingv1.UpdatePublicThingResponse], error) {
	thing, err := update(req.Msg.GetThing())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&testingv1.UpdatePublicThingResponse{Thing: thing}), nil
}

func (testThings) GetAppThing(_ context.Context, req *connect.Request[testingv1.GetAppThingRequest]) (*connect.Response[testingv1.GetAppThingResponse], error) {
	return connect.NewResponse(&testingv1.GetAppThingResponse{Thing: thingFor(req.Msg.GetId())}), nil
}

func (testThings) UpdateAppThing(_ context.Context, req *connect.Request[testingv1.UpdateAppThingRequest]) (*connect.Response[testingv1.UpdateAppThingResponse], error) {
	thing, err := update(req.Msg.GetThing())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&testingv1.UpdateAppThingResponse{Thing: thing}), nil
}

func (testThings) GetAdminThing(_ context.Context, req *connect.Request[testingv1.GetAdminThingRequest]) (*connect.Response[testingv1.GetAdminThingResponse], error) {
	return connect.NewResponse(&testingv1.GetAdminThingResponse{Thing: thingFor(req.Msg.GetId())}), nil
}

func (testThings) UpdateAdminThing(_ context.Context, req *connect.Request[testingv1.UpdateAdminThingRequest]) (*connect.Response[testingv1.UpdateAdminThingResponse], error) {
	thing, err := update(req.Msg.GetThing())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&testingv1.UpdateAdminThingResponse{Thing: thing}), nil
}

func testService[T any](name protoreflect.Name, impl T, newHandler func(T, ...connect.HandlerOption) (string, http.Handler)) rpc.Service {
	return rpc.Service{
		Desc: testingv1.File_app_testing_v1_testing_proto.Services().ByName(name),
		New:  func(opts ...connect.HandlerOption) (string, http.Handler) { return newHandler(impl, opts...) },
	}
}

// classServices are the Public, App and Admin test services.
func classServices() []rpc.Service {
	var impl testThings
	return []rpc.Service{
		testService("TestingPublicService", testingv1connect.TestingPublicServiceHandler(impl), testingv1connect.NewTestingPublicServiceHandler),
		testService("TestingAppService", testingv1connect.TestingAppServiceHandler(impl), testingv1connect.NewTestingAppServiceHandler),
		testService("TestingAdminService", testingv1connect.TestingAdminServiceHandler(impl), testingv1connect.NewTestingAdminServiceHandler),
	}
}

// withTestServices mounts classServices in an authAPI.
func withTestServices(_ *auth.Config, d *Deps) { d.testServices = classServices() }

// handlerTransport serves a client's requests with h, in memory. Requests come from the same origin, as the
// frontend's do, unless the request sets Sec-Fetch-Site itself; cookie, when set, is the session cookie. opts change
// each request last (another peer address, more cookies or headers: auth_client_test.go).
type handlerTransport struct {
	h      http.Handler
	cookie string
	opts   []requestOpt
}

func (tr handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// the peer httptest.NewRequest uses, so raw requests and client calls share a rate-limit bucket
	req.RemoteAddr = "192.0.2.1:1234"
	if req.Header.Get("Sec-Fetch-Site") == "" {
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	if tr.cookie != "" {
		req.Header.Add("Cookie", cookieName+"="+tr.cookie)
	}
	for _, o := range tr.opts {
		o(req)
	}
	rec := httptest.NewRecorder()
	tr.h.ServeHTTP(rec, req)
	return rec.Result(), nil
}

// connectClient is an HTTP client for the generated Connect clients, at baseURL (the API's /api).
func connectClient(h http.Handler, cookie string) *http.Client {
	return &http.Client{Transport: handlerTransport{h: h, cookie: cookie}}
}

const baseURL = "http://app.test/api"

// newConnectAPI is NewHandler with the class test services and no database: every caller is anonymous.
func newConnectAPI(t *testing.T, deps Deps) http.Handler {
	t.Helper()
	if deps.Logger == nil {
		deps.Logger = discard
	}
	deps.Ready = ok
	deps.testServices = append(classServices(), deps.testServices...)
	return newTestHandlerDeps(t, deps)
}

// assertConnectError checks err's code and, when reason is not empty, its ErrorInfo.
func assertConnectError(t *testing.T, err error, code connect.Code, reason string) {
	t.Helper()
	ce := asConnectError(t, err)
	if ce.Code() != code {
		t.Fatalf("code = %s (%v), want %s", ce.Code(), ce, code)
	}
	if reason != "" {
		info := errorInfo(t, ce)
		if info == nil || info.GetReason() != reason || info.GetDomain() != rpc.ErrorDomain {
			t.Fatalf("ErrorInfo = %v, want reason %s in domain %s", info, reason, rpc.ErrorDomain)
		}
	}
}

func asConnectError(t *testing.T, err error) *connect.Error {
	t.Helper()
	var ce *connect.Error
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v, want a Connect error", err)
	}
	return ce
}

func errorDetail[T any](t *testing.T, ce *connect.Error) *T {
	t.Helper()
	for _, d := range ce.Details() {
		v, err := d.Value()
		if err != nil {
			t.Fatal(err)
		}
		if m, ok := any(v).(*T); ok {
			return m
		}
	}
	return nil
}

func errorInfo(t *testing.T, ce *connect.Error) *errdetails.ErrorInfo {
	t.Helper()
	return errorDetail[errdetails.ErrorInfo](t, ce)
}

// connectPost sends body to a procedure as the Connect protocol's unary JSON, the way a browser would.
func connectPost(t *testing.T, h http.Handler, procedure, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://app.test/api"+procedure, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// wireError is a Connect error as it is on the wire (Connect protocol, JSON).
type wireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details []struct {
		Type  string `json:"type"`
		Value string `json:"value"` // the detail's binary Protobuf, unpadded base64
	} `json:"details"`
}

// reason is the ErrorInfo reason the error carries, "" without one.
func (e wireError) reason(t *testing.T) string {
	t.Helper()
	for _, d := range e.Details {
		if d.Type != "google.rpc.ErrorInfo" {
			continue
		}
		b, err := base64.RawStdEncoding.DecodeString(d.Value)
		if err != nil {
			t.Fatalf("ErrorInfo value %q: %v", d.Value, err)
		}
		var info errdetails.ErrorInfo
		if err := proto.Unmarshal(b, &info); err != nil {
			t.Fatalf("ErrorInfo: %v", err)
		}
		return info.GetReason()
	}
	return ""
}

func decodeWireError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) wireError {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d %s, want %d", rec.Code, rec.Body, status)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json (%s)", ct, rec.Body)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("body is not JSON: %v (%q)", err, rec.Body)
	}
	// the request id is only in the X-Request-ID header (B17)
	if _, ok := raw["request_id"]; ok {
		t.Errorf("Connect error body carries request_id: %s", rec.Body)
	}
	if rec.Header().Get(headerRequestID) == "" {
		t.Error("no X-Request-ID header")
	}
	var e wireError
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e.Code != code {
		t.Fatalf("code = %q (%s), want %q", e.Code, rec.Body, code)
	}
	return e
}

func TestConnectPublicServiceThroughNewHandler(t *testing.T) {
	h := newConnectAPI(t, Deps{})
	for name, opts := range map[string][]connect.ClientOption{
		"POST": nil,
		// NO_SIDE_EFFECTS methods are served on GET too, as the frontend calls them
		"GET": {connect.WithHTTPGet()},
	} {
		client := testingv1connect.NewTestingPublicServiceClient(connectClient(h, ""), baseURL, opts...)
		resp, err := client.GetPublicThing(t.Context(), connect.NewRequest(&testingv1.GetPublicThingRequest{Id: "7"}))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := resp.Msg.GetThing().GetId(); got != "7" {
			t.Errorf("%s: thing id = %q, want 7", name, got)
		}
		if got := resp.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control = %q, want no-store", name, got)
		}
	}
}

// The same server answers a rejected cross-origin request in the request's format.
func TestCrossOriginRejectedInEachFormat(t *testing.T) {
	h := newConnectAPI(t, Deps{})
	crossSite := map[string]string{"Sec-Fetch-Site": "cross-site"}
	e := decodeWireError(t, connectPost(t, h, testingv1connect.TestingPublicServiceUpdatePublicThingProcedure, `{}`, crossSite),
		http.StatusForbidden, "permission_denied")
	if len(e.Details) != 1 {
		t.Fatalf("details = %+v, want one ErrorInfo", e.Details)
	}
	client := testingv1connect.NewTestingPublicServiceClient(connectClient(h, ""), baseURL,
		connect.WithInterceptors(connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Sec-Fetch-Site", "cross-site")
				return next(ctx, req)
			}
		})))
	_, err := client.UpdatePublicThing(t.Context(), connect.NewRequest(&testingv1.UpdatePublicThingRequest{}))
	assertConnectError(t, err, connect.CodePermissionDenied, commonv1.ErrorReason_CROSS_ORIGIN_REQUEST.String())
}

// An anonymous caller of an App or Admin method is unauthenticated before connect-go decodes the body: a body it
// cannot decode is invalid_argument only on a Public method.
func TestConnectUnauthenticatedBeforeDecoding(t *testing.T) {
	h := newConnectAPI(t, Deps{})
	for _, procedure := range []string{
		testingv1connect.TestingAppServiceUpdateAppThingProcedure,
		testingv1connect.TestingAdminServiceUpdateAdminThingProcedure,
	} {
		decodeWireError(t, connectPost(t, h, procedure, `{not json`, nil), http.StatusUnauthorized, "unauthenticated")
		rec := connectPost(t, h, procedure, `{}`, map[string]string{"Content-Type": "text/plain"})
		decodeWireError(t, rec, http.StatusUnauthorized, "unauthenticated")
	}
	decodeWireError(t, connectPost(t, h, testingv1connect.TestingPublicServiceUpdatePublicThingProcedure, `{not json`, nil),
		http.StatusBadRequest, "invalid_argument")

	client := testingv1connect.NewTestingAppServiceClient(connectClient(h, ""), baseURL)
	_, err := client.GetAppThing(t.Context(), connect.NewRequest(&testingv1.GetAppThingRequest{Id: "1"}))
	assertConnectError(t, err, connect.CodeUnauthenticated, commonv1.ErrorReason_SESSION_INVALID.String())
}

// A Connect path whose procedure is not mounted is not_found, without asking for a session (TestUnknownPathNotFound
// covers paths that are not a procedure).
func TestConnectUnknownProcedureNotFound(t *testing.T) {
	h := newConnectAPI(t, Deps{})
	for _, procedure := range []string{
		"/app.x.v1.NopeAppService/Get",
		"/app.testing.v1.TestingAppService/Nope",
		"/app.testing.v1.TestingAppService/",
		"/app.",
	} {
		decodeWireError(t, connectPost(t, h, procedure, `{}`, nil), http.StatusNotFound, "not_found")
	}
}

// A body over maxBodyBytes is resource_exhausted without RetryInfo: connect-go turns the MaxBytesReader error into it,
// and the missing RetryInfo is what tells it from the rate limit (B16).
func TestConnectBodyTooLarge(t *testing.T) {
	h := newConnectAPI(t, Deps{})
	body := `{"thing":{"id":"1","name":"` + strings.Repeat("x", maxBodyBytes) + `"}}`
	e := decodeWireError(t, connectPost(t, h, testingv1connect.TestingPublicServiceUpdatePublicThingProcedure, body, nil),
		http.StatusTooManyRequests, "resource_exhausted")
	for _, d := range e.Details {
		if d.Type == "google.rpc.RetryInfo" {
			t.Errorf("a body over the limit carries RetryInfo, which only the rate limit sends")
		}
	}
}

// The entry rate limit runs before decoding: a malformed request takes a token, and over the limit it is
// resource_exhausted with RetryInfo instead of invalid_argument.
func TestConnectRateLimitBeforeDecoding(t *testing.T) {
	h := newConnectAPI(t, Deps{RateLimitPerMin: 2, testLimited: []string{testingv1connect.TestingPublicServiceUpdatePublicThingProcedure}})
	procedure := testingv1connect.TestingPublicServiceUpdatePublicThingProcedure
	for range 2 {
		decodeWireError(t, connectPost(t, h, procedure, `{not json`, nil), http.StatusBadRequest, "invalid_argument")
	}
	decodeWireError(t, connectPost(t, h, procedure, `{not json`, nil), http.StatusTooManyRequests, "resource_exhausted")

	client := testingv1connect.NewTestingPublicServiceClient(connectClient(h, ""), baseURL)
	_, err := client.UpdatePublicThing(t.Context(), connect.NewRequest(&testingv1.UpdatePublicThingRequest{Thing: thingFor("1")}))
	assertConnectError(t, err, connect.CodeResourceExhausted, "")
	ce := asConnectError(t, err)
	if retry := errorDetail[errdetails.RetryInfo](t, ce); retry == nil || retry.GetRetryDelay().AsDuration() <= 0 {
		t.Errorf("RetryInfo = %v, want a positive delay", retry)
	}
	// other procedures keep their own budget
	if _, err := client.GetPublicThing(t.Context(), connect.NewRequest(&testingv1.GetPublicThingRequest{Id: "1"})); err != nil {
		t.Errorf("GetPublicThing: %v", err)
	}
}

func TestConnectRateLimitNamesMountedProcedures(t *testing.T) {
	_, err := NewHandler(Deps{Ready: ok, Logger: discard, Now: time.Now, RateLimitPerMin: 5,
		testServices: classServices(), testLimited: []string{"/app.testing.v1.TestingPublicService/Nope"}})
	if err == nil || !strings.Contains(err.Error(), "Nope") {
		t.Fatalf("NewHandler = %v, want an error naming the procedure", err)
	}
}

// Errors the handler returns reach the client converted: an expected failure keeps its code and reason, anything
// else is internal without its text, which goes to the log at error level.
func TestConnectHandlerErrors(t *testing.T) {
	var buf bytes.Buffer
	h := newConnectAPI(t, Deps{Logger: slog.New(slog.NewJSONHandler(&buf, nil))})
	client := testingv1connect.NewTestingPublicServiceClient(connectClient(h, ""), baseURL)
	updateTo := func(name string) error {
		_, err := client.UpdatePublicThing(t.Context(), connect.NewRequest(&testingv1.UpdatePublicThingRequest{
			Thing: &testingv1.Thing{Id: "1", Name: name},
		}))
		return err
	}

	assertConnectError(t, updateTo("taken"), connect.CodeAlreadyExists, commonv1.ErrorReason_ACCOUNT_TAKEN.String())

	err := updateTo("boom")
	assertConnectError(t, err, connect.CodeInternal, "")
	ce := asConnectError(t, err)
	if strings.Contains(ce.Message(), "hunter2") || len(ce.Details()) > 0 {
		t.Errorf("internal error leaked: %q, %d details", ce.Message(), len(ce.Details()))
	}
	line := findLog(t, logLines(t, &buf), "internal error")
	if line["level"] != "ERROR" || line["procedure"] != testingv1connect.TestingPublicServiceUpdatePublicThingProcedure ||
		!strings.Contains(line["error"].(string), "hunter2") {
		t.Errorf("internal error log = %v", line)
	}

	assertConnectError(t, updateTo("panic"), connect.CodeInternal, "")
	if line := findLog(t, logLines(t, &buf), "panic"); line["level"] != "ERROR" || line["stack"] == nil {
		t.Errorf("panic log = %v", line)
	}
}

// Every service NewHandler mounts belongs to a caller class; anything else stops the server from starting (B10).
func TestNewHandlerRejectsUnclassifiedService(t *testing.T) {
	var impl testThings
	_, err := NewHandler(Deps{Ready: ok, Logger: discard, Now: time.Now, RateLimitPerMin: 5,
		testServices: []rpc.Service{testService("TestingInternalService", testingv1connect.TestingInternalServiceHandler(impl),
			testingv1connect.NewTestingInternalServiceHandler)}})
	if err == nil || !strings.Contains(err.Error(), "TestingInternalService") {
		t.Fatalf("NewHandler = %v, want an error naming TestingInternalService", err)
	}
}

// A method that is not Get* or List* must not be NO_SIDE_EFFECTS: it would be served on GET, which has no CSRF check.
func TestNewHandlerRejectsSideEffectsOnGet(t *testing.T) {
	var impl testThings
	_, err := NewHandler(Deps{Ready: ok, Logger: discard, Now: time.Now, RateLimitPerMin: 5,
		testServices: []rpc.Service{testService("TestingSideEffectAppService", testingv1connect.TestingSideEffectAppServiceHandler(impl),
			testingv1connect.NewTestingSideEffectAppServiceHandler)}})
	if err == nil || !strings.Contains(err.Error(), "UpdateSideEffectThing") || strings.Contains(err.Error(), "GetSideEffectThing") {
		t.Fatalf("NewHandler = %v, want an error naming UpdateSideEffectThing only", err)
	}
}

// resourceProblems checks what replaces the REST rule "every 201 has a Location you can GET" (B23, AIP-121/133/134):
// every Create<X> and Update<X> returns the resource a Get<X> returns (the response's message field of the resource's
// type), and that Get<X> is a method of a mounted service in the same proto package. Not necessarily the same
// service: sign-up and sign-in are Public (UserPublicService.CreateUser, SessionPublicService.CreateSession) and their
// resources are read through App services (UserAppService.GetUser, SessionAppService.GetSession) with the session the
// response sets, as the old followLocation read a 201's Location (TestCreateUserSetsSessionCookie,
// TestCreateSessionReturnsSession). A Get<X> in another package does not count: admin.users.v1's GetUser is not
// users.v1's.
func resourceProblems(services []rpc.Service) []string {
	byPackage := map[protoreflect.FullName][]protoreflect.ServiceDescriptor{}
	for _, s := range services {
		pkg := s.Desc.ParentFile().Package()
		byPackage[pkg] = append(byPackage[pkg], s.Desc)
	}
	var problems []string
	for _, s := range services {
		methods := s.Desc.Methods()
		for i := range methods.Len() {
			if p := resourceProblem(byPackage[s.Desc.ParentFile().Package()], methods.Get(i)); p != "" {
				problems = append(problems, p)
			}
		}
	}
	slices.Sort(problems)
	return problems
}

// resourceProblem is what is wrong with a Create or Update method, "" when nothing is (or md is neither). siblings are
// the mounted services of md's package, md's own included.
func resourceProblem(siblings []protoreflect.ServiceDescriptor, md protoreflect.MethodDescriptor) string {
	name := string(md.Name())
	resource, ok := strings.CutPrefix(name, "Create")
	if !ok {
		resource, ok = strings.CutPrefix(name, "Update")
	}
	if !ok {
		return ""
	}
	hasGet := false
	for _, svc := range siblings {
		get := svc.Methods().ByName(protoreflect.Name("Get" + resource))
		if get == nil {
			continue
		}
		hasGet = true
		if want := resourceType(get.Output()); want != "" && resourceType(md.Output()) == want {
			return ""
		}
	}
	if !hasGet {
		return rpc.Procedure(md) + " has no Get" + resource + " in " + string(md.ParentFile().Package())
	}
	return rpc.Procedure(md) + " does not return the resource Get" + resource + " returns"
}

// resourceType is the full name of the one message-typed field of a response, "" when there is not exactly one.
func resourceType(msg protoreflect.MessageDescriptor) protoreflect.FullName {
	var found protoreflect.FullName
	fields := msg.Fields()
	for i := range fields.Len() {
		f := fields.Get(i)
		if f.Message() == nil || f.IsList() || f.IsMap() {
			continue
		}
		if found != "" {
			return ""
		}
		found = f.Message().FullName()
	}
	return found
}

func TestConnectServicesReturnTheirResources(t *testing.T) {
	services := connectServices(Deps{testServices: classServices()})
	if len(services) == 0 {
		t.Fatal("connectServices mounts nothing: the descriptor checks would pass on an empty set")
	}
	for _, p := range resourceProblems(services) {
		t.Error(p)
	}
}

// the check itself: an Update without a Get, returning no resource, is reported; so is a Create whose Get is only in
// another package, or not mounted
func TestResourceProblemsReportsViolations(t *testing.T) {
	var impl testThings
	got := resourceProblems([]rpc.Service{testService("TestingOrphanAppService", testingv1connect.TestingOrphanAppServiceHandler(impl),
		testingv1connect.NewTestingOrphanAppServiceHandler)})
	want := []string{testingv1connect.TestingOrphanAppServiceUpdateOrphanThingProcedure + " has no GetOrphanThing in app.testing.v1"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	mounted := map[protoreflect.Name]rpc.Service{}
	for _, s := range connectServices(Deps{}) {
		mounted[s.Desc.Name()] = s
	}
	// UserAdminService.GetUser (app.admin.users.v1) does not read back what UserPublicService.CreateUser creates
	got = resourceProblems([]rpc.Service{mounted["UserPublicService"], mounted["UserAdminService"]})
	want = []string{usersv1connect.UserPublicServiceCreateUserProcedure + " has no GetUser in app.users.v1"}
	if !slices.Equal(got, want) {
		t.Fatalf("without UserAppService: got %v, want %v", got, want)
	}
	if got := resourceProblems([]rpc.Service{mounted["UserPublicService"], mounted["UserAppService"]}); len(got) > 0 {
		t.Errorf("with UserAppService: %v", got)
	}
}

// Every code the server sends as ErrorInfo.reason or FieldViolation.reason is declared in app.common.v1, which the
// frontend translates value by value.
func TestErrorCodesDeclaredInProto(t *testing.T) {
	codes := []string{auth.CodeAccountTaken, auth.CodeInvalidCredentials, auth.CodeLoginThrottled, auth.CodeLastAdmin,
		auth.CodeReauthenticationRequired, auth.ErrSessionInvalid.Code, rpc.ErrInsufficientRole.Code, codeCrossOrigin}
	for _, c := range codes {
		if _, ok := commonv1.ErrorReason_value[strings.ToUpper(c)]; !ok {
			t.Errorf("code %s is not an app.common.v1.ErrorReason value", c)
		}
	}
	// field codes: the auth and items limits, ids and paging
	for _, c := range []string{auth.CodePasswordTooCommon, "required", "too_short", "too_long", "pattern", "invalid", "out_of_range"} {
		if _, ok := commonv1.FieldViolationReason_value[strings.ToUpper(c)]; !ok {
			t.Errorf("field code %s is not an app.common.v1.FieldViolationReason value", c)
		}
	}
}
