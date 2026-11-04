package httpapi

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"

	adminusersv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/admin/users/v1"
	itemsv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/items/v1"
	sessionsv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/sessions/v1"
)

// A list request runs as many queries for a full page as for one row: nothing is loaded one row at a time (N+1). The
// lists are called as the frontend calls them: GET (the generated clients use WithHTTPGet), a page of 100.
func TestListQueriesDoNotGrowWithRows(t *testing.T) {
	a := newAuthAPI(t)
	admin := a.mustAdmin()
	_, token := a.mustSignUp("alice")
	lists := map[string]func() error{
		"items": func() error {
			_, err := a.items(token).ListItems(t.Context(), connect.NewRequest(&itemsv1.ListItemsRequest{PageSize: 100}))
			return err
		},
		"sessions": func() error {
			_, err := a.sessions(token).ListSessions(t.Context(), connect.NewRequest(&sessionsv1.ListSessionsRequest{PageSize: 100}))
			return err
		},
		"users": func() error {
			_, err := a.admin(admin).ListUsers(t.Context(), connect.NewRequest(&adminusersv1.ListUsersRequest{PageSize: 100}))
			return err
		},
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
