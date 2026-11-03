package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	commonv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/common/v1"
	testingv1 "github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc/testdata/gen/app/testing/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc/testdata/gen/app/testing/v1/testingv1connect"
)

// The Connect entry with real sessions (the database): the session cookie authenticate resolves reaches App
// services, and only an admin reaches Admin services.

func TestConnectAppServiceWithSession(t *testing.T) {
	a := newAuthAPI(t, withTestServices)
	_, token := a.mustSignUp("alice")
	client := testingv1connect.NewTestingAppServiceClient(connectClient(a.h, token), baseURL, connect.WithHTTPGet())
	resp, err := client.GetAppThing(t.Context(), connect.NewRequest(&testingv1.GetAppThingRequest{Id: "3"}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg.GetThing().GetId() != "3" {
		t.Errorf("thing = %v", resp.Msg.GetThing())
	}
}

func TestConnectAdminServiceNeedsAdminRole(t *testing.T) {
	a := newAuthAPI(t, withTestServices)
	_, user := a.mustSignUp("alice")
	admin := a.mustAdmin()
	req := func() *connect.Request[testingv1.UpdateAdminThingRequest] {
		return connect.NewRequest(&testingv1.UpdateAdminThingRequest{Thing: thingFor("1")})
	}

	_, err := testingv1connect.NewTestingAdminServiceClient(connectClient(a.h, user), baseURL).UpdateAdminThing(t.Context(), req())
	assertConnectError(t, err, connect.CodePermissionDenied, commonv1.ErrorReason_INSUFFICIENT_ROLE.String())

	if _, err := testingv1connect.NewTestingAdminServiceClient(connectClient(a.h, admin), baseURL).UpdateAdminThing(t.Context(), req()); err != nil {
		t.Errorf("admin: %v", err)
	}
}

// A database error while authenticate resolves the session is internal, in the request's format.
func TestAuthenticateDatabaseErrorInEachFormat(t *testing.T) {
	a := newAuthAPI(t, withTestServices)
	_, token := a.mustSignUp("alice")
	a.pool.Close()

	_, err := testingv1connect.NewTestingAppServiceClient(connectClient(a.h, token), baseURL).
		GetAppThing(t.Context(), connect.NewRequest(&testingv1.GetAppThingRequest{Id: "1"}))
	assertConnectError(t, err, connect.CodeInternal, "")
	_, err = a.me(token)
	assertConnectError(t, err, connect.CodeInternal, "")

	// authenticate runs before the split, so a REST path (no operation left; step 5 deletes the REST layer) gets the
	// problem
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://app.test/api/rest-path", http.NoBody)
	req.Header.Set("Cookie", cookieName+"="+token)
	a.h.ServeHTTP(rec, req)
	assertProblem(t, rec, http.StatusInternalServerError, "/problems/internal")
}
