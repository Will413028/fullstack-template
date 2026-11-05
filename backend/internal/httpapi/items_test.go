package httpapi

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	itemsv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/items/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/gen/app/items/v1/itemsv1connect"
)

// items is the generated client of ItemAppService as the user with the session token sees it ("" for anonymous),
// reading with GET as the frontend's transport does.
func (a *authAPI) items(token string) itemsv1connect.ItemAppServiceClient {
	return itemsv1connect.NewItemAppServiceClient(connectClient(a.h, token), baseURL, connect.WithHTTPGet())
}

func (a *authAPI) mustCreateItem(token, title string) *itemsv1.Item {
	a.t.Helper()
	resp, err := a.items(token).CreateItem(a.t.Context(), connect.NewRequest(&itemsv1.CreateItemRequest{
		Item: &itemsv1.Item{Title: title},
	}))
	if err != nil {
		a.t.Fatalf("create item %q: %v", title, err)
	}
	return resp.Msg.GetItem()
}

func (a *authAPI) getItem(token, id string) (*itemsv1.Item, error) {
	a.t.Helper()
	resp, err := a.items(token).GetItem(a.t.Context(), connect.NewRequest(&itemsv1.GetItemRequest{Id: id}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetItem(), nil
}

func (a *authAPI) mustGetItem(token, id string) *itemsv1.Item {
	a.t.Helper()
	it, err := a.getItem(token, id)
	if err != nil {
		a.t.Fatalf("get item %s: %v", id, err)
	}
	return it
}

// updateItem sends item with a mask of paths; no paths sends no mask.
func (a *authAPI) updateItem(token string, item *itemsv1.Item, paths ...string) (*itemsv1.Item, error) {
	a.t.Helper()
	req := &itemsv1.UpdateItemRequest{Item: item}
	if paths != nil {
		req.UpdateMask = &fieldmaskpb.FieldMask{Paths: paths}
	}
	resp, err := a.items(token).UpdateItem(a.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetItem(), nil
}

func (a *authAPI) mustUpdateItem(token string, item *itemsv1.Item, paths ...string) *itemsv1.Item {
	a.t.Helper()
	it, err := a.updateItem(token, item, paths...)
	if err != nil {
		a.t.Fatalf("update item %s %v: %v", item.GetId(), paths, err)
	}
	return it
}

func (a *authAPI) deleteItem(token, id string) error {
	a.t.Helper()
	_, err := a.items(token).DeleteItem(a.t.Context(), connect.NewRequest(&itemsv1.DeleteItemRequest{Id: id}))
	return err
}

func (a *authAPI) listItems(token string, req *itemsv1.ListItemsRequest) (*itemsv1.ListItemsResponse, error) {
	a.t.Helper()
	resp, err := a.items(token).ListItems(a.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func (a *authAPI) mustListItems(token string, req *itemsv1.ListItemsRequest) *itemsv1.ListItemsResponse {
	a.t.Helper()
	page, err := a.listItems(token, req)
	if err != nil {
		a.t.Fatalf("list items %v: %v", req, err)
	}
	return page
}

// assertFieldViolation checks that err is invalid_argument with exactly one field violation, on field with reason.
func assertFieldViolation(t *testing.T, err error, field, reason string) {
	t.Helper()
	assertConnectError(t, err, connect.CodeInvalidArgument, "")
	br := errorDetail[errdetails.BadRequest](t, asConnectError(t, err))
	if br == nil || len(br.GetFieldViolations()) != 1 {
		t.Fatalf("BadRequest = %v, want one violation on %s", br, field)
	}
	if v := br.GetFieldViolations()[0]; v.GetField() != field || v.GetReason() != reason {
		t.Fatalf("violation = %s %s, want %s %s", v.GetField(), v.GetReason(), field, reason)
	}
}

func assertSameItem(t *testing.T, got, want *itemsv1.Item) {
	t.Helper()
	if !proto.Equal(got, want) {
		t.Errorf("item = %v, want %v", got, want)
	}
}

func TestCreateItem(t *testing.T) {
	a := newAuthAPI(t)
	u, token := a.mustSignUp("alice")
	resp, err := a.items(token).CreateItem(t.Context(), connect.NewRequest(&itemsv1.CreateItemRequest{
		Item: &itemsv1.Item{Title: "Buy milk", Description: proto.String("two litres")},
	}))
	if err != nil {
		t.Fatal(err)
	}
	it := resp.Msg.GetItem()
	if it.GetTitle() != "Buy milk" || it.GetDescription() != "two litres" || it.GetOwnerId() != u.GetId() {
		t.Errorf("item = %v, want title, description and owner %s", it, u.Id)
	}
	if _, err := uuid.Parse(it.GetId()); err != nil || !it.GetCreateTime().AsTime().Equal(it.GetUpdateTime().AsTime()) {
		t.Errorf("item = %v, want a UUID id and create_time = update_time", it)
	}
	assertSameItem(t, a.mustGetItem(token, it.GetId()), it)
	// description is optional: unset comes back unset, the empty string comes back as the empty string
	if got := a.mustCreateItem(token, "No description"); got.Description != nil {
		t.Errorf("description = %q, want unset", got.GetDescription())
	}
	resp, err = a.items(token).CreateItem(t.Context(), connect.NewRequest(&itemsv1.CreateItemRequest{
		Item: &itemsv1.Item{Title: "Empty description", Description: proto.String("")},
	}))
	if err != nil || resp.Msg.GetItem().Description == nil {
		t.Errorf("empty description = %v, %v, want set to the empty string", resp, err)
	}
	// the fields the server sets are not the client's to choose
	other := uuid.Must(uuid.NewV7()).String()
	resp, err = a.items(token).CreateItem(t.Context(), connect.NewRequest(&itemsv1.CreateItemRequest{
		Item: &itemsv1.Item{Id: other, Title: "mine", OwnerId: other},
	}))
	if err != nil || resp.Msg.GetItem().GetId() == other || resp.Msg.GetItem().GetOwnerId() != u.GetId() {
		t.Errorf("create with id and owner_id = %v, %v, want both set by the server", resp, err)
	}
}

// Every items method needs a session; an anonymous caller gets unauthenticated before the request is decoded.
func TestItemsNeedSession(t *testing.T) {
	h := newConnectAPI(t, Deps{})
	for _, procedure := range []string{
		itemsv1connect.ItemAppServiceCreateItemProcedure, itemsv1connect.ItemAppServiceGetItemProcedure,
		itemsv1connect.ItemAppServiceListItemsProcedure, itemsv1connect.ItemAppServiceUpdateItemProcedure,
		itemsv1connect.ItemAppServiceDeleteItemProcedure,
	} {
		t.Run(procedure, func(t *testing.T) {
			decodeWireError(t, connectPost(t, h, procedure, `{not json`, nil), http.StatusUnauthorized, "unauthenticated")
		})
	}
	_, err := itemsv1connect.NewItemAppServiceClient(connectClient(h, ""), baseURL, connect.WithHTTPGet()).
		ListItems(t.Context(), connect.NewRequest(&itemsv1.ListItemsRequest{}))
	assertConnectError(t, err, connect.CodeUnauthenticated, "SESSION_INVALID")
}

// Titles are 1 to 200 characters counted in code points, by the items service and again by the database: the old
// backend let a long title reach the database and answered 500.
func TestCreateItemTitleLimits(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	create := func(item *itemsv1.Item) error {
		_, err := a.items(token).CreateItem(t.Context(), connect.NewRequest(&itemsv1.CreateItemRequest{Item: item}))
		return err
	}
	assertFieldViolation(t, create(&itemsv1.Item{Title: strings.Repeat("a", 201)}), "item.title", "TOO_LONG")
	assertFieldViolation(t, create(&itemsv1.Item{Title: ""}), "item.title", "REQUIRED")
	assertFieldViolation(t, create(nil), "item.title", "REQUIRED")
	// 200 multi-byte characters are 600 bytes but 200 code points: accepted by both layers
	if it := a.mustCreateItem(token, strings.Repeat("字", 200)); len([]rune(it.GetTitle())) != 200 {
		t.Errorf("title has %d characters", len([]rune(it.GetTitle())))
	}
	if _, err := a.pool.Exec(t.Context(), "INSERT INTO items (title, owner_id) SELECT repeat('a', 201), id FROM users LIMIT 1"); err == nil {
		t.Error("database accepted a 201-character title")
	}
}

func TestListMyItems(t *testing.T) {
	a := newAuthAPI(t)
	_, alice := a.mustSignUp("alice")
	_, bob := a.mustSignUp("bob")
	first := a.mustCreateItem(alice, "first")
	second := a.mustCreateItem(alice, "second")
	a.mustCreateItem(bob, "bob's")
	page := a.mustListItems(alice, &itemsv1.ListItemsRequest{})
	if len(page.GetItems()) != 2 || !proto.Equal(page.GetItems()[0], second) || !proto.Equal(page.GetItems()[1], first) {
		t.Errorf("items = %v, want alice's two, newest first", page.GetItems())
	}
	if page.GetNextPageToken() != "" {
		t.Errorf("next_page_token = %q, want empty on the only page", page.GetNextPageToken())
	}
	_, carol := a.mustSignUp("carol")
	if page := a.mustListItems(carol, &itemsv1.ListItemsRequest{}); len(page.GetItems()) != 0 || page.GetNextPageToken() != "" {
		t.Errorf("a user without items got %v", page)
	}
}

// Keyset pagination ordered by id: items created while a client pages through do not shift later pages, so nothing
// is returned twice or skipped.
func TestPaginationKeyset(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	var want []string // newest first
	for i := range 5 {
		want = append([]string{a.mustCreateItem(token, "item "+string(rune('a'+i))).GetId()}, want...)
	}
	var got []string
	req := &itemsv1.ListItemsRequest{PageSize: 2}
	for pages := 0; ; pages++ {
		if pages > 5 {
			t.Fatal("pagination did not end")
		}
		page := a.mustListItems(token, req)
		if len(page.GetItems()) > 2 {
			t.Fatalf("page has %d items, page_size 2", len(page.GetItems()))
		}
		for _, it := range page.GetItems() {
			got = append(got, it.GetId())
		}
		if pages == 0 {
			a.mustCreateItem(token, "created while paging")
		}
		if page.GetNextPageToken() == "" {
			break
		}
		req = &itemsv1.ListItemsRequest{PageSize: 2, PageToken: page.GetNextPageToken()}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("paged ids = %v, want %v", got, want)
	}
}

// page_size: 0 is the default 20, more than 100 is read as 100 (AIP-158 coerces), negative is invalid_argument.
// page_token: empty is the first page, one this API did not return is invalid_argument.
func TestListItemsPageParameters(t *testing.T) {
	a := newAuthAPI(t)
	u, token := a.mustSignUp("alice")
	if _, err := a.pool.Exec(t.Context(),
		"INSERT INTO items (title, owner_id) SELECT 'item ' || g, $1 FROM generate_series(1, 101) g", u.Id); err != nil {
		t.Fatal(err)
	}
	for size, want := range map[int32]int{0: 20, 3: 3, 100: 100, 101: 100, 1000: 100} {
		page := a.mustListItems(token, &itemsv1.ListItemsRequest{PageSize: size})
		if len(page.GetItems()) != want || page.GetNextPageToken() == "" {
			t.Errorf("page_size %d: %d items, next_page_token %q, want %d and a token", size, len(page.GetItems()),
				page.GetNextPageToken(), want)
		}
	}
	_, err := a.listItems(token, &itemsv1.ListItemsRequest{PageSize: -1})
	assertFieldViolation(t, err, "page_size", "OUT_OF_RANGE")
	_, err = a.listItems(token, &itemsv1.ListItemsRequest{PageToken: "not-a-token"})
	assertFieldViolation(t, err, "page_token", "INVALID")
}

func TestGetItem(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	it := a.mustCreateItem(token, "mine")
	assertSameItem(t, a.mustGetItem(token, it.GetId()), it)
	_, err := a.getItem(token, uuid.Must(uuid.NewV7()).String())
	assertConnectError(t, err, connect.CodeNotFound, "")
	_, err = a.getItem(token, "42")
	assertConnectError(t, err, connect.CodeNotFound, "")
}

// Regression (the old backend let anyone read any item): another user's item is not_found, exactly like one that
// does not exist.
func TestGetOtherUsersItem(t *testing.T) {
	a := newAuthAPI(t)
	_, alice := a.mustSignUp("alice")
	_, bob := a.mustSignUp("bob")
	it := a.mustCreateItem(alice, "alice's")
	_, err := a.getItem(bob, it.GetId())
	assertConnectError(t, err, connect.CodeNotFound, "")
}

func TestUpdateItem(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	it := a.mustCreateItem(token, "old")
	got := a.mustUpdateItem(token, &itemsv1.Item{Id: it.GetId(), Title: "new", Description: proto.String("details")},
		"title", "description")
	if got.GetTitle() != "new" || got.GetDescription() != "details" || got.GetId() != it.GetId() ||
		!got.GetCreateTime().AsTime().Equal(it.GetCreateTime().AsTime()) {
		t.Errorf("after UpdateItem = %v", got)
	}
	assertSameItem(t, a.mustGetItem(token, it.GetId()), got)
	_, err := a.updateItem(token, &itemsv1.Item{Id: uuid.Must(uuid.NewV7()).String(), Title: "x"}, "title")
	assertConnectError(t, err, connect.CodeNotFound, "")
	_, err = a.updateItem(token, &itemsv1.Item{Id: "42", Title: "x"}, "title")
	assertConnectError(t, err, connect.CodeNotFound, "")
}

// update_mask (AIP-134): only the listed fields change, a listed description that is unset clears it, no mask
// changes the fields that have a value, "*" is every field, and a listed title must not be empty.
func TestUpdateItemMask(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	id := a.mustUpdateItem(token, &itemsv1.Item{Id: a.mustCreateItem(token, "title").GetId(),
		Description: proto.String("keep me")}, "description").GetId()
	check := func(got *itemsv1.Item, title string, description *string) {
		t.Helper()
		if got.GetTitle() != title || (got.Description == nil) != (description == nil) ||
			(description != nil && got.GetDescription() != *description) {
			t.Errorf("item = %v, want title %q, description %v", got, title, description)
		}
	}

	// the mask lists title only: the description in the request is not applied
	check(a.mustUpdateItem(token, &itemsv1.Item{Id: id, Title: "renamed", Description: proto.String("ignored")}, "title"),
		"renamed", proto.String("keep me"))
	check(a.mustUpdateItem(token, &itemsv1.Item{Id: id, Title: "ignored", Description: proto.String("changed")}, "description"),
		"renamed", proto.String("changed"))
	// without a mask, the fields that have a value change: here only title
	check(a.mustUpdateItem(token, &itemsv1.Item{Id: id, Title: "no mask"}), "no mask", proto.String("changed"))
	check(a.mustUpdateItem(token, &itemsv1.Item{Id: id, Title: "renamed"}, "title", "update_time", "owner_id"),
		"renamed", proto.String("changed"))
	check(a.mustUpdateItem(token, &itemsv1.Item{Id: id}, "description"), "renamed", nil)
	check(a.mustUpdateItem(token, &itemsv1.Item{Id: id}), "renamed", nil)
	check(a.mustUpdateItem(token, &itemsv1.Item{Id: id, Title: "both", Description: proto.String("set")}, "*"),
		"both", proto.String("set"))

	_, err := a.updateItem(token, &itemsv1.Item{Id: id}, "title")
	assertFieldViolation(t, err, "item.title", "REQUIRED")
	_, err = a.updateItem(token, &itemsv1.Item{Id: id}, "*")
	assertFieldViolation(t, err, "item.title", "REQUIRED")
	_, err = a.updateItem(token, &itemsv1.Item{Id: id, Title: strings.Repeat("a", 201)}, "title")
	assertFieldViolation(t, err, "item.title", "TOO_LONG")
	_, err = a.updateItem(token, &itemsv1.Item{Id: id, Title: "x"}, "titel")
	assertFieldViolation(t, err, "update_mask", "INVALID")
	check(a.mustGetItem(token, id), "both", proto.String("set"))
}

// update_time is set by the update query itself, not by the caller or a trigger.
func TestUpdatedAtChanges(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	it := a.mustCreateItem(token, "title")
	time.Sleep(10 * time.Millisecond)
	got := a.mustUpdateItem(token, &itemsv1.Item{Id: it.GetId(), Title: "changed"}, "title")
	if !got.GetUpdateTime().AsTime().After(it.GetUpdateTime().AsTime()) || !got.GetCreateTime().AsTime().Equal(it.GetCreateTime().AsTime()) {
		t.Errorf("after UpdateItem: create_time %v -> %v, update_time %v -> %v", it.GetCreateTime().AsTime(),
			got.GetCreateTime().AsTime(), it.GetUpdateTime().AsTime(), got.GetUpdateTime().AsTime())
	}
	// a mask that changes nothing returns the item unchanged, update_time included
	if same := a.mustUpdateItem(token, &itemsv1.Item{Id: it.GetId()}); !proto.Equal(same, got) {
		t.Errorf("empty update = %v, want %v", same, got)
	}
}

func TestDeleteItem(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	it := a.mustCreateItem(token, "doomed")
	if err := a.deleteItem(token, it.GetId()); err != nil {
		t.Fatal(err)
	}
	_, err := a.getItem(token, it.GetId())
	assertConnectError(t, err, connect.CodeNotFound, "")
	assertConnectError(t, a.deleteItem(token, it.GetId()), connect.CodeNotFound, "")
	assertConnectError(t, a.deleteItem(token, "42"), connect.CodeNotFound, "")
}

// Changing or deleting another user's item is not_found, not permission_denied, and leaves it untouched.
func TestUpdateOtherUsersItem(t *testing.T) {
	a := newAuthAPI(t)
	_, alice := a.mustSignUp("alice")
	_, bob := a.mustSignUp("bob")
	it := a.mustCreateItem(alice, "alice's")
	_, err := a.updateItem(bob, &itemsv1.Item{Id: it.GetId(), Title: "bob was here"}, "title")
	assertConnectError(t, err, connect.CodeNotFound, "")
	assertConnectError(t, a.deleteItem(bob, it.GetId()), connect.CodeNotFound, "")
	assertSameItem(t, a.mustGetItem(alice, it.GetId()), it)
}

// CSRF (docs/adr/2026-10-18-api-and-auth-baseline.md D5): a cross-site state-changing request is rejected before it
// reaches the handler.
func TestCreateItemCrossSite(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	_, err := itemsv1connect.NewItemAppServiceClient(&http.Client{Transport: crossSiteTransport{handlerTransport{h: a.h, cookie: token}}},
		baseURL).CreateItem(t.Context(), connect.NewRequest(&itemsv1.CreateItemRequest{Item: &itemsv1.Item{Title: "csrf"}}))
	assertConnectError(t, err, connect.CodePermissionDenied, "CROSS_ORIGIN_REQUEST")
	if page := a.mustListItems(token, &itemsv1.ListItemsRequest{}); len(page.GetItems()) != 0 {
		t.Errorf("cross-site request created %d items", len(page.GetItems()))
	}
}

// crossSiteTransport sends every request as a browser does from another site.
type crossSiteTransport struct{ next http.RoundTripper }

func (c crossSiteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	return c.next.RoundTrip(req)
}
