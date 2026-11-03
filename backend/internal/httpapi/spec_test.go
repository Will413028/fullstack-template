package httpapi

import (
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Will413028/template-go-nextjs/backend/api"
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

// docs/adr/2026-10-18-api-and-auth-baseline.md D9: a resource a client can change or delete can also be read. The
// contract has no paths left (every module is on Connect, where TestConnectServicesReturnTheirResources checks the
// same rule); this keeps the REST check honest until step 5 deletes it with the contract.
func TestEveryWritablePathHasGet(t *testing.T) {
	spec, err := loadSpec(t.Context(), api.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if missing := writableWithoutGet(spec); len(missing) > 0 {
		t.Errorf("paths with PATCH, PUT or DELETE but no GET: %v", missing)
	}
	// the check itself: a copy of the contract with a path that has DELETE but no GET must fail it
	broken, err := loadSpec(t.Context(), api.Spec)
	if err != nil {
		t.Fatal(err)
	}
	broken.Paths.Set("/widgets/{id}", &openapi3.PathItem{Delete: &openapi3.Operation{}})
	if missing := writableWithoutGet(broken); !slices.Equal(missing, []string{"/widgets/{id}"}) {
		t.Errorf("with DELETE /widgets/{id} only: missing = %v, want [/widgets/{id}]", missing)
	}
}
