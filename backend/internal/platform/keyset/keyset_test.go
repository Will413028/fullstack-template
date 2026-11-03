package keyset

import (
	"testing"

	"github.com/google/uuid"
)

func TestTrim(t *testing.T) {
	ids := []uuid.UUID{uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())}
	self := func(id uuid.UUID) uuid.UUID { return id }
	if Rows(2) != 3 {
		t.Fatalf("Rows(2) = %d, want 3", Rows(2))
	}
	page, next := Trim(ids, 2, self)
	if len(page) != 2 || next == nil || *next != ids[1] {
		t.Errorf("Trim(3 rows, 2) = %v, %v; want the first two and the second's id", page, next)
	}
	page, next = Trim(ids[:2], 2, self)
	if len(page) != 2 || next != nil {
		t.Errorf("Trim(2 rows, 2) = %v, %v; want both and no next page", page, next)
	}
	if page, next = Trim(ids, 0, self); len(page) != 0 || next != nil {
		t.Errorf("Trim(3 rows, 0) = %v, %v; want nothing", page, next)
	}
}
