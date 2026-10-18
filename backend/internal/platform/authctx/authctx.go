// Package authctx carries the authenticated user through context.Context, so modules learn who is calling without
// depending on the HTTP layer (plan I4). internal/httpapi puts the user in after resolving the session cookie.
package authctx

import (
	"context"

	"github.com/google/uuid"
)

// User is the caller. Role is read from the database on every request, so a change takes effect immediately.
type User struct {
	ID   uuid.UUID
	Role string
}

type key struct{}

// With returns ctx carrying u.
func With(ctx context.Context, u User) context.Context { return context.WithValue(ctx, key{}, u) }

// From returns the user in ctx, if any.
func From(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(key{}).(User)
	return u, ok
}
