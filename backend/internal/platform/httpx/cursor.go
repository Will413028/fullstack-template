// Package httpx holds HTTP-shaped helpers that modules share without importing net/http: the keyset pagination
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

func invalidCursor() error {
	return apperr.Invalid("/cursor", "invalid", "cursor is not one this API returned")
}
