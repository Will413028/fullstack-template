package httpapi

import (
	"context"

	"github.com/oapi-codegen/nullable"

	"github.com/Will413028/fullstack-template/backend/internal/httpapi/gen"
	"github.com/Will413028/fullstack-template/backend/internal/modules/items"
)

// The items operations (/items). The items service scopes every query to the signed-in user.

// defaultPageSize is the Limit parameter's default in api/openapi.yaml; the validator does not fill defaults in.
// TestDefaultPageSizeMatchesContract keeps the two equal.
const defaultPageSize = 20

// optional turns a pointer into a nullable value that is always set: Item and ItemPage declare these fields required,
// and an unset nullable.Nullable would marshal as "" instead of null.
func optional[T any](p *T) nullable.Nullable[T] {
	if p == nil {
		return nullable.NewNullNullable[T]()
	}
	return nullable.NewNullableWithValue(*p)
}

// nextCursor is a page's next_cursor: null on the last page, where the service returns no cursor.
func nextCursor(cursor string) nullable.Nullable[string] {
	if cursor == "" {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(cursor)
}

// pageSize is the Limit parameter, or its default when the client sent none.
func pageSize(limit *int) int {
	if limit == nil {
		return defaultPageSize
	}
	return *limit
}

// fromNullable returns the value of a nullable field the client sent, nil for null or omitted.
func fromNullable[T any](n nullable.Nullable[T]) *T {
	if !n.IsSpecified() || n.IsNull() {
		return nil
	}
	v := n.MustGet()
	return &v
}

func toItem(it items.Item) gen.Item {
	return gen.Item{
		Id: it.ID, Title: it.Title, Description: optional(it.Description), OwnerId: it.OwnerID,
		CreatedAt: it.CreatedAt.UTC(), UpdatedAt: it.UpdatedAt.UTC(),
	}
}

func (s *apiServer) CreateItem(ctx context.Context, req gen.CreateItemRequestObject) (gen.CreateItemResponseObject, error) {
	it, err := s.items.Create(ctx, req.Body.Title, fromNullable(req.Body.Description))
	if err != nil {
		return nil, err
	}
	location := "/api/items/" + it.ID.String()
	return gen.CreateItem201JSONResponse{Body: toItem(it), Headers: gen.CreateItem201ResponseHeaders{Location: &location}}, nil
}

func (s *apiServer) ListItems(ctx context.Context, req gen.ListItemsRequestObject) (gen.ListItemsResponseObject, error) {
	// An empty cursor is not one this API returned: 422, not the first page.
	page, err := s.items.List(ctx, pageSize(req.Params.Limit), req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	resp := gen.ListItems200JSONResponse{Items: make([]gen.Item, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for _, it := range page.Items {
		resp.Items = append(resp.Items, toItem(it))
	}
	return resp, nil
}

func (s *apiServer) GetItem(ctx context.Context, req gen.GetItemRequestObject) (gen.GetItemResponseObject, error) {
	it, err := s.items.Get(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.GetItem200JSONResponse(toItem(it)), nil
}

func (s *apiServer) UpdateItem(ctx context.Context, req gen.UpdateItemRequestObject) (gen.UpdateItemResponseObject, error) {
	p := items.Patch{Title: req.Body.Title, SetDescription: req.Body.Description.IsSpecified(), Description: fromNullable(req.Body.Description)}
	it, err := s.items.Update(ctx, req.Id, p)
	if err != nil {
		return nil, err
	}
	return gen.UpdateItem200JSONResponse(toItem(it)), nil
}

func (s *apiServer) DeleteItem(ctx context.Context, req gen.DeleteItemRequestObject) (gen.DeleteItemResponseObject, error) {
	if err := s.items.Delete(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.DeleteItem204Response{}, nil
}
