// Package keyset is keyset pagination that modules share: read one row more than a page to know whether another
// page follows, and the id the next page starts after. The page token clients see is internal/httpapi/rpc's.
package keyset

import "github.com/google/uuid"

// Rows is the row limit for a page of limit items: one more, so Trim knows whether another page follows without a
// second query or a count.
func Rows(limit int) int32 {
	return int32(limit) + 1 //nolint:gosec // limit is 1..100 (rpc.PageSize)
}

// Trim cuts rows read with Rows(limit), newest first, to the page and returns the id of its last row when another
// page follows: nil when no extra row came back, so the last page has no next page.
func Trim[T any](rows []T, limit int, id func(T) uuid.UUID) ([]T, *uuid.UUID) {
	if len(rows) <= limit {
		return rows, nil
	}
	last := id(rows[limit-1])
	return rows[:limit], &last
}
