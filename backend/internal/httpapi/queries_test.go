package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A list request runs as many queries for a full page as for one row: nothing is loaded one row at a time (N+1).
func TestListQueriesDoNotGrowWithRows(t *testing.T) {
	a := newAuthAPI(t)
	admin := a.mustAdmin()
	_, token := a.mustSignUp("alice")
	lists := map[string]func() *httptest.ResponseRecorder{
		"items": func() *httptest.ResponseRecorder { _, rec := a.listItems(token, "?limit=100"); return rec },
		"sessions": func() *httptest.ResponseRecorder {
			return a.do(http.MethodGet, "/api/sessions?limit=100", token, "", "")
		},
		"users": func() *httptest.ResponseRecorder { return a.do(http.MethodGet, "/api/users?limit=100", admin, "", "") },
	}
	few := map[string]int64{}
	for name, list := range lists {
		few[name] = a.countQueries(list)
	}
	for i := range 10 {
		a.mustCreateItem(token, fmt.Sprintf("item %d", i))
		a.mustSignIn("alice", goodPassword)
		a.mustSignUp(fmt.Sprintf("user%d", i))
	}
	for name, list := range lists {
		if got := a.countQueries(list); got != few[name] {
			t.Errorf("listing %s ran %d queries with 10 more rows, %d before", name, got, few[name])
		}
	}
}
