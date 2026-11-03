package httpapi

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	itemsv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/items/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/gen/app/items/v1/itemsv1connect"
	"github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc"
	"github.com/Will413028/template-go-nextjs/backend/internal/modules/items"
)

// itemsHandler serves app.items.v1.ItemAppService (api/app/items/v1/items.proto). The items service scopes every
// query to the signed-in user and checks the limits; this handler maps the messages and the field paths.
type itemsHandler struct {
	items *items.Service
}

// Page sizes of ListItems, as the proto documents them.
const (
	defaultItemsPage = 20
	maxItemsPage     = 100
)

// itemWritable are the Item fields a caller may set; the others are the server's.
var itemWritable = []string{"title", "description"}

func itemsService(svc *items.Service) rpc.Service {
	return rpc.Service{
		Desc: itemsv1.File_app_items_v1_items_proto.Services().ByName("ItemAppService"),
		New: func(opts ...connect.HandlerOption) (string, http.Handler) {
			return itemsv1connect.NewItemAppServiceHandler(itemsHandler{items: svc}, opts...)
		},
	}
}

func toItem(it items.Item) *itemsv1.Item {
	return &itemsv1.Item{
		Id: it.ID.String(), Title: it.Title, Description: it.Description, OwnerId: it.OwnerID.String(),
		CreateTime: timestamppb.New(it.CreatedAt), UpdateTime: timestamppb.New(it.UpdatedAt),
	}
}

func (h itemsHandler) CreateItem(ctx context.Context, req *connect.Request[itemsv1.CreateItemRequest]) (*connect.Response[itemsv1.CreateItemResponse], error) {
	in := req.Msg.GetItem()
	if in == nil {
		in = &itemsv1.Item{} // no item: the service reports the missing title
	}
	it, err := h.items.Create(ctx, in.GetTitle(), in.Description)
	if err != nil {
		return nil, rpc.Under("item", err)
	}
	return connect.NewResponse(&itemsv1.CreateItemResponse{Item: toItem(it)}), nil
}

func (h itemsHandler) GetItem(ctx context.Context, req *connect.Request[itemsv1.GetItemRequest]) (*connect.Response[itemsv1.GetItemResponse], error) {
	id, err := rpc.ParseID("id", req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	it, err := h.items.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&itemsv1.GetItemResponse{Item: toItem(it)}), nil
}

func (h itemsHandler) ListItems(ctx context.Context, req *connect.Request[itemsv1.ListItemsRequest]) (*connect.Response[itemsv1.ListItemsResponse], error) {
	size, cursor, err := rpc.PageRequest(req.Msg.GetPageSize(), req.Msg.GetPageToken(), defaultItemsPage, maxItemsPage)
	if err != nil {
		return nil, err
	}
	page, err := h.items.List(ctx, size, cursor)
	if err != nil {
		return nil, err
	}
	resp := &itemsv1.ListItemsResponse{Items: make([]*itemsv1.Item, 0, len(page.Items)), NextPageToken: page.NextCursor}
	for _, it := range page.Items {
		resp.Items = append(resp.Items, toItem(it))
	}
	return connect.NewResponse(resp), nil
}

func (h itemsHandler) UpdateItem(ctx context.Context, req *connect.Request[itemsv1.UpdateItemRequest]) (*connect.Response[itemsv1.UpdateItemResponse], error) {
	in := req.Msg.GetItem()
	if in == nil {
		in = &itemsv1.Item{}
	}
	id, err := rpc.ParseID("id", in.GetId())
	if err != nil {
		return nil, rpc.Under("item", err)
	}
	paths, err := rpc.UpdatePaths(req.Msg.GetUpdateMask(), in, itemWritable...)
	if err != nil {
		return nil, err
	}
	var p items.Patch
	if paths["title"] {
		title := in.GetTitle()
		p.Title = &title
	}
	if paths["description"] {
		p.SetDescription, p.Description = true, in.Description
	}
	it, err := h.items.Update(ctx, id, p)
	if err != nil {
		return nil, rpc.Under("item", err)
	}
	return connect.NewResponse(&itemsv1.UpdateItemResponse{Item: toItem(it)}), nil
}

func (h itemsHandler) DeleteItem(ctx context.Context, req *connect.Request[itemsv1.DeleteItemRequest]) (*connect.Response[itemsv1.DeleteItemResponse], error) {
	id, err := rpc.ParseID("id", req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if err := h.items.Delete(ctx, id); err != nil {
		return nil, err
	}
	return connect.NewResponse(&itemsv1.DeleteItemResponse{}), nil
}
