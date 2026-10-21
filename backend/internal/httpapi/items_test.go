package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Will413028/fullstack-template/backend/api"
	"github.com/Will413028/fullstack-template/backend/internal/httpapi/gen"
)

func (a *authAPI) createItem(token, body string) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.do(http.MethodPost, "/api/items", token, "application/json", body)
}

func (a *authAPI) mustCreateItem(token, title string) gen.Item {
	a.t.Helper()
	rec := a.createItem(token, mustJSON(a.t, map[string]string{"title": title}))
	if rec.Code != http.StatusCreated {
		a.t.Fatalf("create item %q = %d %s", title, rec.Code, rec.Body)
	}
	return decodeItem(a.t, rec)
}

func (a *authAPI) getItem(token string, id uuid.UUID) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.do(http.MethodGet, "/api/items/"+id.String(), token, "", "")
}

func (a *authAPI) patchItem(token string, id uuid.UUID, body string) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.do(http.MethodPatch, "/api/items/"+id.String(), token, "application/merge-patch+json", body)
}

func (a *authAPI) deleteItem(token string, id uuid.UUID) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.do(http.MethodDelete, "/api/items/"+id.String(), token, "", "")
}

func decodeItem(t *testing.T, rec *httptest.ResponseRecorder) gen.Item {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var it gen.Item
	if err := json.Unmarshal(rec.Body.Bytes(), &it); err != nil {
		t.Fatalf("body is not an Item: %v (%q)", err, rec.Body)
	}
	return it
}

// itemFields decodes an Item as a plain map, to see null and missing fields the way a client does.
func itemFields(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body is not a JSON object: %v (%q)", err, rec.Body)
	}
	return m
}

func (a *authAPI) listItems(token, query string) (gen.ItemPage, *httptest.ResponseRecorder) {
	a.t.Helper()
	rec := a.do(http.MethodGet, "/api/items"+query, token, "", "")
	var page gen.ItemPage
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			a.t.Fatalf("body is not an ItemPage: %v (%q)", err, rec.Body)
		}
	}
	return page, rec
}

func TestCreateItem(t *testing.T) {
	a := newAuthAPI(t)
	u, token := a.mustSignUp("alice")
	rec := a.createItem(token, `{"title":"Buy milk","description":"two litres"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/items = %d %s", rec.Code, rec.Body)
	}
	it := decodeItem(t, rec)
	if it.Title != "Buy milk" || it.Description.MustGet() != "two litres" || it.OwnerId != u.Id {
		t.Errorf("item = %+v, want title, description and owner %s", it, u.Id)
	}
	if loc := rec.Header().Get("Location"); loc != "/api/items/"+it.Id.String() {
		t.Errorf("Location = %q", loc)
	}
	if got := a.getItem(token, it.Id); got.Code != http.StatusOK || decodeItem(t, got).Id != it.Id {
		t.Errorf("GET Location = %d %s", got.Code, got.Body)
	}
	// description is optional and comes back as null, not missing or ""
	m := itemFields(t, a.createItem(token, `{"title":"No description"}`))
	if v, ok := m["description"]; !ok || v != nil {
		t.Errorf("description = %v (present %v), want null", v, ok)
	}
}

func TestCreateItemUnauthenticated(t *testing.T) {
	a := newAuthAPI(t)
	assertProblem(t, a.createItem("", `{"title":"x"}`), http.StatusUnauthorized, "/problems/unauthenticated")
}

// Titles are 1 to 200 characters counted in code points, by the contract and again by the database: the old backend
// let a long title reach the database and answered 500.
func TestCreateItemTitleTooLong(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	p := assertProblem(t, a.createItem(token, mustJSON(t, map[string]string{"title": strings.Repeat("a", 201)})),
		http.StatusUnprocessableEntity, "/problems/validation")
	assertFieldError(t, p, "/title", "too_long")
	p = assertProblem(t, a.createItem(token, `{"title":""}`), http.StatusUnprocessableEntity, "/problems/validation")
	assertFieldError(t, p, "/title", "too_short")
	p = assertProblem(t, a.createItem(token, `{"description":"no title"}`), http.StatusUnprocessableEntity, "/problems/validation")
	assertFieldError(t, p, "/title", "required")
	// 200 multi-byte characters are 600 bytes but 200 code points: accepted by both layers
	if it := a.mustCreateItem(token, strings.Repeat("字", 200)); len([]rune(it.Title)) != 200 {
		t.Errorf("title has %d characters", len([]rune(it.Title)))
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
	page, rec := a.listItems(alice, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/items = %d %s", rec.Code, rec.Body)
	}
	if len(page.Items) != 2 || page.Items[0].Id != second.Id || page.Items[1].Id != first.Id {
		t.Errorf("items = %+v, want alice's two, newest first", page.Items)
	}
	if !page.NextCursor.IsNull() {
		t.Errorf("next_cursor = %v, want null on the only page", page.NextCursor)
	}
	// a user without items gets an empty array, not null
	_, carol := a.mustSignUp("carol")
	_, rec = a.listItems(carol, "")
	if m := itemFields(t, rec); m["items"] == nil || m["next_cursor"] != nil {
		t.Errorf("empty page = %s", rec.Body)
	}
	assertProblem(t, a.do(http.MethodGet, "/api/items", "", "", ""), http.StatusUnauthorized, "/problems/unauthenticated")
}

// Keyset pagination ordered by id: items created while a client pages through do not shift later pages, so nothing
// is returned twice or skipped.
func TestPaginationKeyset(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	var want []uuid.UUID // newest first
	for i := range 5 {
		want = append([]uuid.UUID{a.mustCreateItem(token, "item "+string(rune('a'+i))).Id}, want...)
	}
	var got []uuid.UUID
	query := "?limit=2"
	for pages := 0; ; pages++ {
		if pages > 5 {
			t.Fatal("pagination did not end")
		}
		page, rec := a.listItems(token, query)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/items%s = %d %s", query, rec.Code, rec.Body)
		}
		if len(page.Items) > 2 {
			t.Fatalf("page has %d items, limit 2", len(page.Items))
		}
		for _, it := range page.Items {
			got = append(got, it.Id)
		}
		if pages == 0 {
			a.mustCreateItem(token, "created while paging")
		}
		if page.NextCursor.IsNull() {
			break
		}
		query = "?limit=2&cursor=" + page.NextCursor.MustGet()
	}
	if !slices.Equal(got, want) {
		t.Fatalf("paged ids = %v, want %v", got, want)
	}
}

func TestListItemsInvalidPage(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	p := assertProblem(t, a.do(http.MethodGet, "/api/items?limit=101", token, "", ""), http.StatusUnprocessableEntity, "/problems/validation")
	assertFieldError(t, p, "/limit", "out_of_range")
	p = assertProblem(t, a.do(http.MethodGet, "/api/items?cursor=not-a-cursor", token, "", ""), http.StatusUnprocessableEntity, "/problems/validation")
	assertFieldError(t, p, "/cursor", "invalid")
	p = assertProblem(t, a.do(http.MethodGet, "/api/items?cursor=", token, "", ""), http.StatusUnprocessableEntity, "/problems/validation")
	assertFieldError(t, p, "/cursor", "invalid")
}

// The handler applies the contract's default page size itself (the validator does not fill defaults in).
func TestDefaultPageSizeMatchesContract(t *testing.T) {
	spec, err := loadSpec(t.Context(), api.Spec)
	if err != nil {
		t.Fatal(err)
	}
	def := spec.Components.Parameters["Limit"].Value.Schema.Value.Default
	if n, ok := def.(float64); !ok || int(n) != defaultPageSize {
		t.Errorf("contract Limit default = %v, defaultPageSize = %d", def, defaultPageSize)
	}
}

// kin-openapi parses integers with base 0 and lets 0x10 through; the generated binder rejects it, and writeParamError
// turns that into the same 422 a contract violation gets (this test is the end-to-end check of that path).
func TestListItemsLimitNotDecimal(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	p := assertProblem(t, a.do(http.MethodGet, "/api/items?limit=0x10", token, "", ""), http.StatusUnprocessableEntity, "/problems/validation")
	assertFieldError(t, p, "/limit", "invalid_type")
}

func TestGetItem(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	it := a.mustCreateItem(token, "mine")
	rec := a.getItem(token, it.Id)
	if rec.Code != http.StatusOK || !reflect.DeepEqual(decodeItem(t, rec), it) {
		t.Errorf("GET = %d %s, want %+v", rec.Code, rec.Body, it)
	}
	assertProblem(t, a.getItem(token, uuid.Must(uuid.NewV7())), http.StatusNotFound, "/problems/not-found")
	p := assertProblem(t, a.do(http.MethodGet, "/api/items/42", token, "", ""), http.StatusUnprocessableEntity, "/problems/validation")
	assertFieldError(t, p, "/id", "format")
}

// Regression (the old backend let anyone read any item): another user's item is a 404, exactly like one that does not
// exist.
func TestGetOtherUsersItem(t *testing.T) {
	a := newAuthAPI(t)
	_, alice := a.mustSignUp("alice")
	_, bob := a.mustSignUp("bob")
	it := a.mustCreateItem(alice, "alice's")
	assertProblem(t, a.getItem(bob, it.Id), http.StatusNotFound, "/problems/not-found")
}

func TestUpdateItem(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	it := a.mustCreateItem(token, "old")
	rec := a.patchItem(token, it.Id, `{"title":"new","description":"details"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH = %d %s", rec.Code, rec.Body)
	}
	got := decodeItem(t, rec)
	if got.Title != "new" || got.Description.MustGet() != "details" || got.Id != it.Id || !got.CreatedAt.Equal(it.CreatedAt) {
		t.Errorf("after PATCH = %+v", got)
	}
	if again := decodeItem(t, a.getItem(token, it.Id)); !reflect.DeepEqual(again, got) {
		t.Errorf("GET after PATCH = %+v, want %+v", again, got)
	}
	assertProblem(t, a.patchItem(token, uuid.Must(uuid.NewV7()), `{"title":"x"}`), http.StatusNotFound, "/problems/not-found")
}

// JSON Merge Patch (RFC 7396): omitted fields stay, description: null clears, title: null is rejected, and
// only application/merge-patch+json is accepted.
func TestUpdateItemMergePatch(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	rec := a.createItem(token, `{"title":"title","description":"keep me"}`)
	it := decodeItem(t, rec)

	got := decodeItem(t, a.patchItem(token, it.Id, `{"title":"renamed"}`))
	if got.Title != "renamed" || got.Description.MustGet() != "keep me" {
		t.Errorf("omitted description changed: %+v", got)
	}
	got = decodeItem(t, a.patchItem(token, it.Id, `{"description":"changed"}`))
	if got.Title != "renamed" || got.Description.MustGet() != "changed" {
		t.Errorf("omitted title changed: %+v", got)
	}
	rec = a.patchItem(token, it.Id, `{"description":null}`)
	if m := itemFields(t, rec); rec.Code != http.StatusOK || m["description"] != nil || m["title"] != "renamed" {
		t.Errorf("description: null = %d %s, want description cleared", rec.Code, rec.Body)
	}
	if m := itemFields(t, a.patchItem(token, it.Id, `{}`)); m["description"] != nil || m["title"] != "renamed" {
		t.Errorf("empty patch changed the item: %v", m)
	}

	p := assertProblem(t, a.patchItem(token, it.Id, `{"title":null}`), http.StatusUnprocessableEntity, "/problems/validation")
	assertFieldError(t, p, "/title", "null_not_allowed")
	rec = a.do(http.MethodPatch, "/api/items/"+it.Id.String(), token, "application/json", `{"title":"x"}`)
	assertProblem(t, rec, http.StatusUnsupportedMediaType, "/problems/unsupported-media-type")
	if m := itemFields(t, a.getItem(token, it.Id)); m["title"] != "renamed" {
		t.Errorf("rejected patches changed the item: %v", m)
	}
}

// updated_at is set by the update query itself, not by the caller or a trigger.
func TestUpdatedAtChanges(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	it := a.mustCreateItem(token, "title")
	if !it.UpdatedAt.Equal(it.CreatedAt) {
		t.Errorf("new item: created_at %v, updated_at %v", it.CreatedAt, it.UpdatedAt)
	}
	time.Sleep(10 * time.Millisecond)
	got := decodeItem(t, a.patchItem(token, it.Id, `{"title":"changed"}`))
	if !got.UpdatedAt.After(it.UpdatedAt) || !got.CreatedAt.Equal(it.CreatedAt) {
		t.Errorf("after PATCH: created_at %v -> %v, updated_at %v -> %v", it.CreatedAt, got.CreatedAt, it.UpdatedAt, got.UpdatedAt)
	}
}

func TestDeleteItem(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	it := a.mustCreateItem(token, "doomed")
	rec := a.deleteItem(token, it.Id)
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("DELETE = %d %q, want 204 with no body", rec.Code, rec.Body)
	}
	assertProblem(t, a.getItem(token, it.Id), http.StatusNotFound, "/problems/not-found")
	assertProblem(t, a.deleteItem(token, it.Id), http.StatusNotFound, "/problems/not-found")
}

// Changing or deleting another user's item is a 404, not a 403, and leaves it untouched.
func TestUpdateOtherUsersItem(t *testing.T) {
	a := newAuthAPI(t)
	_, alice := a.mustSignUp("alice")
	_, bob := a.mustSignUp("bob")
	it := a.mustCreateItem(alice, "alice's")
	assertProblem(t, a.patchItem(bob, it.Id, `{"title":"bob was here"}`), http.StatusNotFound, "/problems/not-found")
	assertProblem(t, a.deleteItem(bob, it.Id), http.StatusNotFound, "/problems/not-found")
	if got := decodeItem(t, a.getItem(alice, it.Id)); !reflect.DeepEqual(got, it) {
		t.Errorf("alice's item = %+v, want unchanged %+v", got, it)
	}
}

// CSRF (docs/adr/2026-10-18-api-and-auth-baseline.md D5): a cross-site state-changing request is rejected before it
// reaches the handler.
func TestCreateItemCrossSite(t *testing.T) {
	a := newAuthAPI(t)
	_, token := a.mustSignUp("alice")
	rec := a.do(http.MethodPost, "/api/items", token, "application/json", `{"title":"csrf"}`, "Sec-Fetch-Site", "cross-site")
	assertProblem(t, rec, http.StatusForbidden, "/problems/forbidden")
	if page, _ := a.listItems(token, ""); len(page.Items) != 0 {
		t.Errorf("cross-site request created %d items", len(page.Items))
	}
}
