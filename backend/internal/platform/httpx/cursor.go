// Package httpx holds HTTP-shaped helpers that modules share without importing net/http: keyset pagination and its
// cursor.
package httpx

import (
	"encoding/base64"

	"github.com/google/uuid"

	"github.com/Will413028/fullstack-template/backend/internal/platform/apperr"
)

// EncodeCursor returns the opaque cursor for a page that ends at id: its 16 bytes in unpadded base64url. Clients
// pass it back unchanged and never build or parse one.
func EncodeCursor(id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString(id[:])
}

// DecodeCursor returns the id a cursor from EncodeCursor holds. Anything else is a validation error on the cursor
// query parameter.
func DecodeCursor(cursor string) (uuid.UUID, error) {
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return uuid.Nil, invalidCursor()
	}
	id, err := uuid.FromBytes(b)
	if err != nil {
		return uuid.Nil, invalidCursor()
	}
	return id, nil
}

// After decodes the optional cursor parameter: nil for the first page.
func After(cursor *string) (*uuid.UUID, error) {
	if cursor == nil {
		return nil, nil
	}
	id, err := DecodeCursor(*cursor)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// PageRows is the row limit for a page of limit items: one more, so TrimPage knows whether another page follows
// without a second query or a count.
func PageRows(limit int) int32 {
	return int32(limit) + 1 //nolint:gosec // limit is 1..100 (contract)
}

// TrimPage cuts rows read with PageRows(limit), newest first, to the page and returns the cursor for the next page:
// empty when no extra row came back, so the last page has no cursor.
func TrimPage[T any](rows []T, limit int, id func(T) uuid.UUID) ([]T, string) {
	if len(rows) <= limit {
		return rows, ""
	}
	return rows[:limit], EncodeCursor(id(rows[limit-1]))
}

func invalidCursor() error {
	return apperr.Invalid("/cursor", "invalid", "cursor is not one this API returned")
}
