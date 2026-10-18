package authctx

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestWithFrom(t *testing.T) {
	if _, ok := From(context.Background()); ok {
		t.Fatal("empty context has a user")
	}
	u := User{ID: uuid.Must(uuid.NewV7()), Role: "admin"}
	if got, ok := From(With(context.Background(), u)); !ok || got != u {
		t.Fatalf("From = %+v, %v; want %+v", got, ok, u)
	}
}
