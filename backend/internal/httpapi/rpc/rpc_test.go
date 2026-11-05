package rpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/reflect/protoreflect"

	testingv1 "github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc/testdata/gen/app/testing/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc/testdata/gen/app/testing/v1/testingv1connect"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/apperr"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/authctx"
)

var discard = slog.New(slog.DiscardHandler)

func detail[T any](t *testing.T, ce *connect.Error) *T {
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

func TestClassOf(t *testing.T) {
	for name, want := range map[string]Class{
		"app.items.v1.ItemPublicService":      Public,
		"app.items.v1.ItemAppService":         App,
		"app.admin.users.v1.UserAdminService": Admin,
		"app.items.v1.ItemService":            0,
		"app.items.v1.ItemInternalService":    0,
	} {
		got, ok := ClassOf(protoreflect.FullName(name))
		if got != want || ok != (want != 0) {
			t.Errorf("ClassOf(%s) = %v, %v; want %v", name, got, ok, want)
		}
	}
}

func TestConvert(t *testing.T) {
	for _, tc := range []struct {
		err    error
		code   connect.Code
		reason string
	}{
		{apperr.New(apperr.NotFound, "", "no such item"), connect.CodeNotFound, ""},
		{apperr.New(apperr.AlreadyExists, "account_taken", ""), connect.CodeAlreadyExists, "ACCOUNT_TAKEN"},
		{apperr.New(apperr.FailedPrecondition, "last_admin", ""), connect.CodeFailedPrecondition, "LAST_ADMIN"},
		{apperr.New(apperr.Unauthenticated, "invalid_credentials", ""), connect.CodeUnauthenticated, "INVALID_CREDENTIALS"},
		{apperr.New(apperr.Forbidden, "reauthentication_required", ""), connect.CodePermissionDenied, "REAUTHENTICATION_REQUIRED"},
		{fmt.Errorf("wrapped: %w", apperr.New(apperr.Forbidden, "insufficient_role", "")), connect.CodePermissionDenied, "INSUFFICIENT_ROLE"},
		{connect.NewError(connect.CodeUnavailable, errors.New("down")), connect.CodeUnavailable, ""},
		{fmt.Errorf("read body: %w", &http.MaxBytesError{Limit: 1}), connect.CodeResourceExhausted, ""},
	} {
		c := Convert(tc.err)
		if c.Err.Code() != tc.code || c.Internal || len(c.Undeclared) > 0 {
			t.Errorf("%v: %+v, want code %s", tc.err, c, tc.code)
			continue
		}
		info := detail[errdetails.ErrorInfo](t, c.Err)
		if got := info.GetReason(); got != tc.reason {
			t.Errorf("%v: reason = %q, want %q", tc.err, got, tc.reason)
		}
		if info != nil && info.GetDomain() != ErrorDomain {
			t.Errorf("%v: domain = %q", tc.err, info.GetDomain())
		}
	}
}

func TestConvertRateLimited(t *testing.T) {
	c := Convert(&apperr.Error{Kind: apperr.RateLimited, Code: "login_throttled", RetryAfter: 90 * time.Second})
	if c.Err.Code() != connect.CodeResourceExhausted {
		t.Fatalf("code = %s", c.Err.Code())
	}
	if r := detail[errdetails.RetryInfo](t, c.Err); r.GetRetryDelay().AsDuration() != 90*time.Second {
		t.Errorf("RetryInfo = %v, want 90s", r)
	}
	if info := detail[errdetails.ErrorInfo](t, c.Err); info.GetReason() != "LOGIN_THROTTLED" {
		t.Errorf("ErrorInfo = %v", info)
	}
	// the entry rate limit has no code: RetryInfo only
	c = Convert(&apperr.Error{Kind: apperr.RateLimited, RetryAfter: time.Second})
	if detail[errdetails.ErrorInfo](t, c.Err) != nil || detail[errdetails.RetryInfo](t, c.Err) == nil {
		t.Errorf("details = %v, want RetryInfo only", c.Err.Details())
	}
}

func TestConvertValidation(t *testing.T) {
	c := Convert(apperr.Invalid("thing.name", "too_long", "name is over 200 characters"))
	if c.Err.Code() != connect.CodeInvalidArgument {
		t.Fatalf("code = %s", c.Err.Code())
	}
	br := detail[errdetails.BadRequest](t, c.Err)
	if br == nil || len(br.GetFieldViolations()) != 1 {
		t.Fatalf("BadRequest = %v", br)
	}
	v := br.GetFieldViolations()[0]
	if v.GetField() != "thing.name" || v.GetReason() != "TOO_LONG" || v.GetDescription() != "name is over 200 characters" {
		t.Errorf("violation = %v", v)
	}
	// several invalid fields joined are all reported, in order
	c = Convert(errors.Join(apperr.Invalid("account", "too_short", "a"), apperr.Invalid("password", "too_long", "p")))
	got := detail[errdetails.BadRequest](t, c.Err).GetFieldViolations()
	if c.Err.Code() != connect.CodeInvalidArgument || len(got) != 2 || got[0].GetField() != "account" ||
		got[0].GetReason() != "TOO_SHORT" || got[1].GetField() != "password" || got[1].GetReason() != "TOO_LONG" {
		t.Errorf("violations = %v", got)
	}
	// a join wrapped by the service is the same
	wrapped := fmt.Errorf("create user: %w", errors.Join(apperr.Invalid("account", "too_short", ""), apperr.Invalid("password", "too_long", "")))
	if got := detail[errdetails.BadRequest](t, Convert(wrapped).Err).GetFieldViolations(); len(got) != 2 {
		t.Errorf("wrapped join: violations = %v, want account and password", got)
	}
	// a join that is not only invalid fields is an unexpected error
	if c = Convert(errors.Join(apperr.Invalid("account", "too_short", ""), errors.New("db down"))); !c.Internal {
		t.Errorf("mixed join = %v, want internal", c.Err)
	}
}

// A code the proto does not declare is never sent as a reason the client cannot translate; the caller logs it.
func TestConvertUndeclaredCodes(t *testing.T) {
	c := Convert(apperr.New(apperr.Forbidden, "brewing", ""))
	if c.Err.Code() != connect.CodePermissionDenied || detail[errdetails.ErrorInfo](t, c.Err) != nil {
		t.Errorf("undeclared reason sent: %v", c.Err.Details())
	}
	if len(c.Undeclared) != 1 || c.Undeclared[0] != "brewing" {
		t.Errorf("Undeclared = %v", c.Undeclared)
	}
	c = Convert(apperr.Invalid("a", "too_hot", ""))
	if v := detail[errdetails.BadRequest](t, c.Err).GetFieldViolations()[0]; v.GetReason() != "INVALID" {
		t.Errorf("reason = %q, want the fallback INVALID", v.GetReason())
	}
	if len(c.Undeclared) != 1 {
		t.Errorf("Undeclared = %v", c.Undeclared)
	}
	var buf bytes.Buffer
	LogConverted(t.Context(), slog.New(slog.NewJSONHandler(&buf, nil)), "POST", "/p", c.Err, c)
	if !strings.Contains(buf.String(), "too_hot") || !strings.Contains(buf.String(), `"level":"ERROR"`) {
		t.Errorf("log = %s", buf.String())
	}
}

func TestConvertUnexpectedIsInternal(t *testing.T) {
	for _, err := range []error{
		errors.New("pq: password=secret"),
		&apperr.Error{Kind: apperr.Kind(99), Code: "x"},
		// a Connect error the handler wrapped or forwarded: internal and unknown never pass through with their text
		fmt.Errorf("load: %w", connect.NewError(connect.CodeInternal, errors.New("pq: password=secret"))),
		connect.NewError(connect.CodeUnknown, errors.New("upstream said secret")),
	} {
		c := Convert(err)
		if c.Err.Code() != connect.CodeInternal || !c.Internal || strings.Contains(c.Err.Message(), "secret") {
			t.Errorf("%v: %+v", err, c)
		}
	}
}

// things implements the class test services for the mux tests.
type things struct{}

func (things) GetAdminThing(_ context.Context, req *connect.Request[testingv1.GetAdminThingRequest]) (*connect.Response[testingv1.GetAdminThingResponse], error) {
	return connect.NewResponse(&testingv1.GetAdminThingResponse{Thing: &testingv1.Thing{Id: req.Msg.GetId()}}), nil
}

func (things) UpdateAdminThing(_ context.Context, req *connect.Request[testingv1.UpdateAdminThingRequest]) (*connect.Response[testingv1.UpdateAdminThingResponse], error) {
	if req.Msg.GetThing().GetName() == "boom" {
		return nil, errors.New("query failed: password=hunter2")
	}
	return connect.NewResponse(&testingv1.UpdateAdminThingResponse{Thing: req.Msg.GetThing()}), nil
}

func adminService() Service {
	return Service{
		Desc: testingv1.File_app_testing_v1_testing_proto.Services().ByName("TestingAdminService"),
		New: func(opts ...connect.HandlerOption) (string, http.Handler) {
			return testingv1connect.NewTestingAdminServiceHandler(things{}, opts...)
		},
	}
}

// asUser serves m with u signed in, the way internal/httpapi's authenticate puts the user in the context.
func asUser(m http.Handler, u *authctx.User) *http.Client {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u != nil {
			r = r.WithContext(authctx.With(r.Context(), *u))
		}
		m.ServeHTTP(w, r)
	})
	return &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Result(), nil
	})}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAdminServiceNeedsAdminRole(t *testing.T) {
	var buf bytes.Buffer
	m, err := NewMux(slog.New(slog.NewJSONHandler(&buf, nil)), "admin", []Service{adminService()})
	if err != nil {
		t.Fatal(err)
	}
	call := func(u *authctx.User) error {
		_, err := testingv1connect.NewTestingAdminServiceClient(asUser(m, u), "http://app.test").
			UpdateAdminThing(t.Context(), connect.NewRequest(&testingv1.UpdateAdminThingRequest{Thing: &testingv1.Thing{Id: "1"}}))
		return err
	}
	user := authctx.User{ID: uuid.Must(uuid.NewV7()), Role: "user"}
	var ce *connect.Error
	if err := call(&user); !errors.As(err, &ce) || ce.Code() != connect.CodePermissionDenied ||
		detail[errdetails.ErrorInfo](t, ce).GetReason() != "INSUFFICIENT_ROLE" {
		t.Errorf("user: %v, want permission_denied INSUFFICIENT_ROLE", err)
	}
	if !strings.Contains(buf.String(), `"event":"authz.denied"`) || !strings.Contains(buf.String(), user.ID.String()) {
		t.Errorf("no authz.denied log: %s", buf.String())
	}
	if err := call(nil); !errors.As(err, &ce) || ce.Code() != connect.CodeUnauthenticated {
		t.Errorf("anonymous: %v, want unauthenticated", err)
	}
	if err := call(&authctx.User{ID: uuid.Must(uuid.NewV7()), Role: "admin"}); err != nil {
		t.Errorf("admin: %v", err)
	}
}

// convertErrors logs an internal error's cause at error level, once; the client gets only the code.
func TestInternalErrorLogged(t *testing.T) {
	var buf bytes.Buffer
	m, err := NewMux(slog.New(slog.NewJSONHandler(&buf, nil)), "admin", []Service{adminService()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = testingv1connect.NewTestingAdminServiceClient(asUser(m, &authctx.User{Role: "admin"}), "http://app.test").
		UpdateAdminThing(t.Context(), connect.NewRequest(&testingv1.UpdateAdminThingRequest{Thing: &testingv1.Thing{Name: "boom"}}))
	var ce *connect.Error
	if !errors.As(err, &ce) || ce.Code() != connect.CodeInternal || strings.Contains(ce.Message(), "hunter2") {
		t.Fatalf("err = %v", err)
	}
	log := buf.String()
	if strings.Count(log, `"msg":"internal error"`) != 1 || !strings.Contains(log, `"method":"POST"`) || !strings.Contains(log, `"level":"ERROR"`) ||
		!strings.Contains(log, "hunter2") || !strings.Contains(log, testingv1connect.TestingAdminServiceUpdateAdminThingProcedure) {
		t.Errorf("log = %s", log)
	}
}

func TestPageSize(t *testing.T) {
	for _, tc := range []struct {
		requested int32
		want      int
	}{{0, 20}, {1, 1}, {100, 100}, {101, 100}} {
		if got, err := PageSize(tc.requested, 20, 100); err != nil || got != tc.want {
			t.Errorf("PageSize(%d) = %d, %v; want %d", tc.requested, got, err, tc.want)
		}
	}
	if _, err := PageSize(-1, 20, 100); !isInvalid(err, "page_size") {
		t.Errorf("PageSize(-1) = %v", err)
	}
}

func TestPageAfter(t *testing.T) {
	if after, err := PageAfter(""); after != nil || err != nil {
		t.Errorf("empty token = %v, %v; want the first page", after, err)
	}
	id := uuid.Must(uuid.NewV7())
	if after, err := PageAfter(PageToken(&id)); err != nil || *after != id {
		t.Errorf("token = %v, %v; want %s", after, err, id)
	}
	if PageToken(nil) != "" {
		t.Errorf("PageToken(nil) = %q, want empty (the last page)", PageToken(nil))
	}
	valid := PageToken(&id)
	for _, bad := range []string{"not base64!", "AAAA", valid + "=", valid[:len(valid)-2], base64.URLEncoding.EncodeToString(id[:])} {
		if _, err := PageAfter(bad); !isInvalid(err, "page_token") {
			t.Errorf("PageAfter(%q) = %v", bad, err)
		}
	}
}

func TestParseID(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	if got, err := ParseID("item", id.String()); err != nil || got != id {
		t.Errorf("ParseID = %v, %v", got, err)
	}
	// an id that is not a UUID names no resource, so it is the same not_found as one that does not exist
	_, err := ParseID("item", "7")
	if e, ok := apperr.As(err); !ok || e.Kind != apperr.NotFound || e.Detail != "item not found" {
		t.Errorf("ParseID(7) = %v, want item not found", err)
	}
}

func isInvalid(err error, field string) bool {
	e, ok := apperr.As(err)
	return ok && e.Kind == apperr.Validation && e.Field == field
}

type unimplemented struct {
	testingv1connect.UnimplementedTestingInternalServiceHandler
	testingv1connect.UnimplementedTestingSideEffectAppServiceHandler
}

// NewMux refuses a service whose name gives no class (B10) and a NO_SIDE_EFFECTS method other than Get* or List*.
func TestNewMuxRejects(t *testing.T) {
	file := testingv1.File_app_testing_v1_testing_proto.Services()
	for name, s := range map[string]Service{
		"TestingInternalService": {Desc: file.ByName("TestingInternalService"), New: func(o ...connect.HandlerOption) (string, http.Handler) {
			return testingv1connect.NewTestingInternalServiceHandler(unimplemented{}, o...)
		}},
		"UpdateSideEffectThing": {Desc: file.ByName("TestingSideEffectAppService"), New: func(o ...connect.HandlerOption) (string, http.Handler) {
			return testingv1connect.NewTestingSideEffectAppServiceHandler(unimplemented{}, o...)
		}},
	} {
		if _, err := NewMux(discard, "admin", []Service{adminService(), s}); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("NewMux = %v, want an error naming %s", err, name)
		}
	}
	if _, err := NewMux(discard, "admin", []Service{adminService()}); err != nil {
		t.Errorf("a valid service was refused: %v", err)
	}
}

func TestIsRead(t *testing.T) {
	for name, want := range map[string]bool{
		"GetThing": true, "ListThings": true, "Get": false, "List": false, "Getaway": false, "Listen": false,
		"GetOrCreateThing": false, "ListOrRefreshThings": false, "UpdateThing": false, "GetOrder": true,
	} {
		if got := isRead(protoreflect.Name(name)); got != want {
			t.Errorf("isRead(%s) = %v, want %v", name, got, want)
		}
	}
}

// PageRequest is AIP-158 paging for a list whose service takes a size and a cursor.
func TestPageRequest(t *testing.T) {
	if n, cursor, err := PageRequest(0, "", 20, 100); err != nil || n != 20 || cursor != nil {
		t.Errorf("first page = %d, %v, %v; want 20, nil", n, cursor, err)
	}
	if n, _, err := PageRequest(1000, "", 20, 100); err != nil || n != 100 {
		t.Errorf("page_size 1000 = %d, %v; want 100", n, err)
	}
	if _, _, err := PageRequest(-1, "", 20, 100); !isInvalid(err, "page_size") {
		t.Errorf("negative page_size = %v", err)
	}
	if _, _, err := PageRequest(0, "not a token", 20, 100); !isInvalid(err, "page_token") {
		t.Errorf("bad page_token = %v", err)
	}
	id := uuid.Must(uuid.NewV7())
	if _, after, err := PageRequest(0, PageToken(&id), 20, 100); err != nil || after == nil || *after != id {
		t.Errorf("page_token = %v, %v; want %s", after, err, id)
	}
}
