package rpc

import (
	"github.com/google/uuid"

	"github.com/Will413028/template-go-nextjs/backend/internal/platform/apperr"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/httpx"
)

// Pagination follows AIP-158: a request has page_size and page_token, a response next_page_token, which is empty on
// the last page. The token is the keyset cursor internal/platform/httpx encodes (httpx.TrimPage returns it).

// PageSize is the number of items to return for a requested page_size: def when it is 0 (unset), max when it is
// larger (AIP-158 coerces instead of rejecting), and an invalid_argument on page_size when it is negative.
func PageSize(requested int32, def, maxSize int) (int, error) {
	switch {
	case requested < 0:
		return 0, apperr.Invalid("/page_size", "out_of_range", "page_size must not be negative")
	case requested == 0:
		return def, nil
	case int(requested) > maxSize:
		return maxSize, nil
	}
	return int(requested), nil
}

// PageAfter is the id a page_token holds: nil for the first page (the empty token), an invalid_argument on page_token
// for a token this API did not return.
func PageAfter(token string) (*uuid.UUID, error) {
	if token == "" {
		return nil, nil
	}
	id, err := httpx.DecodeCursor(token)
	if err != nil {
		return nil, apperr.Invalid("/page_token", "invalid", "page_token is not one this API returned")
	}
	return &id, nil
}

// ParseID parses a UUID from the request field at path (for example item_id), an invalid_argument on that field
// otherwise.
func ParseID(path, s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, apperr.Invalid("/"+path, "invalid", path+" is not a UUID")
	}
	return id, nil
}
