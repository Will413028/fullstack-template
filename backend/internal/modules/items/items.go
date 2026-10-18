// Package items is the example CRUD module: items owned by a user, the items table (schema.sql) and the queries over
// it (queries/, generated into internal/store by sqlc).
//
// The owner is always the caller from authctx, and every query is scoped to it (plan I6): another user's item is
// reported as not found, never as forbidden, so ids reveal nothing about other users' data.
package items

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Will413028/fullstack-template/backend/internal/modules/items/internal/store"
	"github.com/Will413028/fullstack-template/backend/internal/platform/apperr"
	"github.com/Will413028/fullstack-template/backend/internal/platform/authctx"
	"github.com/Will413028/fullstack-template/backend/internal/platform/httpx"
)

// Item is an item as the API shows it.
type Item struct {
	ID          uuid.UUID
	Title       string
	Description *string
	OwnerID     uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Page is one page of items, newest first. NextCursor is empty on the last page.
type Page struct {
	Items      []Item
	NextCursor string
}

// Patch is a JSON Merge Patch of an item: nil Title leaves the title unchanged; SetDescription says whether
// description was sent, in which case a nil Description clears it.
type Patch struct {
	Title          *string
	SetDescription bool
	Description    *string
}

func (p Patch) empty() bool { return p.Title == nil && !p.SetDescription }

// Service implements items.
type Service struct {
	pool *pgxpool.Pool
}

// New returns the service.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) q() *store.Queries { return store.New(s.pool) }

var errNotFound = apperr.New(apperr.NotFound, "", "item not found")

// owner is the caller. The HTTP layer requires a session for every items operation before calling the service, so
// a missing user is a wiring bug, not a client error.
func owner(ctx context.Context) (uuid.UUID, error) {
	u, ok := authctx.From(ctx)
	if !ok {
		return uuid.Nil, errors.New("items: no authenticated user in context")
	}
	return u.ID, nil
}

// Create creates an item owned by the caller.
func (s *Service) Create(ctx context.Context, title string, description *string) (Item, error) {
	ownerID, err := owner(ctx)
	if err != nil {
		return Item{}, err
	}
	row, err := s.q().CreateItem(ctx, store.CreateItemParams{Title: title, Description: description, OwnerID: ownerID})
	if err != nil {
		return Item{}, fmt.Errorf("items: create: %w", err)
	}
	return Item(row), nil
}

// Get returns the caller's item with id.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Item, error) {
	ownerID, err := owner(ctx)
	if err != nil {
		return Item{}, err
	}
	row, err := s.q().GetItem(ctx, store.GetItemParams{ID: id, OwnerID: ownerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, errNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("items: get: %w", err)
	}
	return Item(row), nil
}

// List returns up to limit of the caller's items after cursor (nil for the first page). It reads one row more
// than limit to know whether another page follows, so the last page has no cursor and no extra round trip.
func (s *Service) List(ctx context.Context, limit int, cursor *string) (Page, error) {
	ownerID, err := owner(ctx)
	if err != nil {
		return Page{}, err
	}
	params := store.ListItemsParams{OwnerID: ownerID, RowLimit: int32(limit) + 1} //nolint:gosec // limit is 1..100 (contract)
	if cursor != nil {
		after, err := httpx.DecodeCursor(*cursor)
		if err != nil {
			return Page{}, err
		}
		params.After = &after
	}
	rows, err := s.q().ListItems(ctx, params)
	if err != nil {
		return Page{}, fmt.Errorf("items: list: %w", err)
	}
	page := Page{Items: make([]Item, 0, min(len(rows), limit))}
	for i, row := range rows {
		if i == limit {
			page.NextCursor = httpx.EncodeCursor(page.Items[limit-1].ID)
			break
		}
		page.Items = append(page.Items, Item(row))
	}
	return page, nil
}

// Update applies p to the caller's item with id. An empty patch returns the item unchanged, updated_at included.
func (s *Service) Update(ctx context.Context, id uuid.UUID, p Patch) (Item, error) {
	if p.empty() {
		return s.Get(ctx, id)
	}
	ownerID, err := owner(ctx)
	if err != nil {
		return Item{}, err
	}
	row, err := s.q().UpdateItem(ctx, store.UpdateItemParams{
		Title: p.Title, SetDescription: p.SetDescription, Description: p.Description, ID: id, OwnerID: ownerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, errNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("items: update: %w", err)
	}
	return Item(row), nil
}

// Delete deletes the caller's item with id.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	ownerID, err := owner(ctx)
	if err != nil {
		return err
	}
	n, err := s.q().DeleteItem(ctx, store.DeleteItemParams{ID: id, OwnerID: ownerID})
	if err != nil {
		return fmt.Errorf("items: delete: %w", err)
	}
	if n == 0 {
		return errNotFound
	}
	return nil
}
