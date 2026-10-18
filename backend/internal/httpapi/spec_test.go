package httpapi

import (
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Will413028/fullstack-template/backend/api"
)

// writableWithoutGet lists the paths in spec that can be changed (PATCH, PUT or DELETE) but not read.
func writableWithoutGet(spec *openapi3.T) []string {
	var missing []string
	for path, item := range spec.Paths.Map() {
		if item.Get == nil && (item.Patch != nil || item.Put != nil || item.Delete != nil) {
			missing = append(missing, path)
		}
	}
	slices.Sort(missing)
	return missing
}

// Plan I7: a resource a client can change or delete can also be read. The runtime half, every 201's Location
// answering GET, is checked on every request the integration tests send (authAPI.followLocation).
func TestEveryWritablePathHasGet(t *testing.T) {
	spec, err := loadSpec(t.Context(), api.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if missing := writableWithoutGet(spec); len(missing) > 0 {
		t.Errorf("paths with PATCH, PUT or DELETE but no GET: %v", missing)
	}
	// the check itself: a copy of the contract without GET /items/{id} must fail it
	broken, err := loadSpec(t.Context(), api.Spec)
	if err != nil {
		t.Fatal(err)
	}
	broken.Paths.Value("/items/{id}").Get = nil
	if missing := writableWithoutGet(broken); !slices.Equal(missing, []string{"/items/{id}"}) {
		t.Errorf("without GET /items/{id}: missing = %v, want [/items/{id}]", missing)
	}
}
