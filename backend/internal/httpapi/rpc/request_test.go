package rpc

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	testingv1 "github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc/testdata/gen/app/testing/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/apperr"
)

// UpdatePaths is AIP-134 over the writable fields: no mask is the populated ones, "*" all of them, the other fields of
// the resource are ignored, and a path that is not a field of the resource is invalid_argument on update_mask.
func TestUpdatePaths(t *testing.T) {
	named := &testingv1.Thing{Id: "1", Name: "n"}
	for _, tc := range []struct {
		name  string
		thing *testingv1.Thing
		paths []string
		want  []string
	}{
		{"no mask, populated", named, nil, []string{"name"}},
		{"no mask, empty", &testingv1.Thing{Id: "1"}, nil, []string{}},
		{"empty mask is no mask", named, []string{}, []string{"name"}},
		{"listed and empty", &testingv1.Thing{Id: "1"}, []string{"name"}, []string{"name"}},
		{"star", &testingv1.Thing{}, []string{"*"}, []string{"name"}},
		{"not writable is ignored", named, []string{"id"}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mask *fieldmaskpb.FieldMask
			if tc.paths != nil {
				mask = &fieldmaskpb.FieldMask{Paths: tc.paths}
			}
			got, err := UpdatePaths(mask, tc.thing, "name")
			if err != nil {
				t.Fatal(err)
			}
			if keys := slices.Sorted(maps.Keys(got)); !slices.Equal(keys, tc.want) {
				t.Errorf("paths = %v, want %v", keys, tc.want)
			}
		})
	}
	for _, bad := range []string{"nmae", "name.sub", ""} {
		if _, err := UpdatePaths(&fieldmaskpb.FieldMask{Paths: []string{bad}}, named, "name"); !isInvalid(err, "update_mask") {
			t.Errorf("path %q: err = %v, want invalid_argument on update_mask", bad, err)
		}
	}
}

// A writable name that is not a field of the resource is a bug in the handler, not a client error.
func TestUpdatePathsUnknownWritablePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic for a writable field the resource does not have")
		}
	}()
	_, _ = UpdatePaths(nil, &testingv1.Thing{}, "nmae")
}

// Under puts a module's fields (title) under the request field that holds them (item.title); other errors pass as
// they are.
func TestUnder(t *testing.T) {
	err := Under("item", apperr.Invalid("title", "too_long", "too long"))
	if !isInvalid(err, "item.title") {
		t.Errorf("Under = %v, want invalid on item.title", err)
	}
	if got := Convert(err).Err; detail[errdetails.BadRequest](t, got).GetFieldViolations()[0].GetField() != "item.title" {
		t.Errorf("field = %v, want item.title", got)
	}
	both := Under("item", errors.Join(apperr.Invalid("title", "required", ""), apperr.Invalid("description", "too_long", "")))
	vs := detail[errdetails.BadRequest](t, Convert(both).Err).GetFieldViolations()
	if len(vs) != 2 || vs[0].GetField() != "item.title" || vs[1].GetField() != "item.description" {
		t.Errorf("joined = %v, want item.title and item.description", vs)
	}
	notFound := apperr.New(apperr.NotFound, "", "nope")
	if got := Under("item", notFound); !errors.Is(got, notFound) {
		t.Errorf("Under(not found) = %v, want it unchanged", got)
	}
	plain := errors.New("db down")
	if got := Under("item", plain); !errors.Is(got, plain) {
		t.Errorf("Under(plain) = %v, want it unchanged", got)
	}
	if Under("item", nil) != nil {
		t.Error("Under(nil) is not nil")
	}
}
