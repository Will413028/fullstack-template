package rpc

import (
	"errors"
	"fmt"
	"maps"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

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
		return 0, apperr.Invalid("page_size", "out_of_range", "page_size must not be negative")
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
		return nil, apperr.Invalid("page_token", "invalid", "page_token is not one this API returned")
	}
	return &id, nil
}

// ParseID parses a UUID from the request field at path (for example item_id), an invalid_argument on that field
// otherwise.
func ParseID(path, s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, apperr.Invalid(path, "invalid", path+" is not a UUID")
	}
	return id, nil
}

// UpdatePaths is the set of fields an Update changes (AIP-134), out of writable, the resource's top-level fields a
// caller may set:
//   - no mask (or an empty one): the writable fields that are populated, so clearing a field needs the mask;
//   - "*": every writable field;
//   - a path that is a field of the resource but not writable (an id, a time the server sets) is ignored;
//   - any other path, nested ones included, is an invalid_argument on update_mask.
//
// A writable name that is not a field of the resource is a bug in the handler: UpdatePaths panics.
func UpdatePaths(mask *fieldmaskpb.FieldMask, resource proto.Message, writable ...string) (map[string]bool, error) {
	m := resource.ProtoReflect()
	fields := m.Descriptor().Fields()
	canWrite := make(map[string]bool, len(writable))
	for _, name := range writable {
		if fields.ByName(protoreflect.Name(name)) == nil {
			panic(fmt.Sprintf("rpc: %s has no field %q", m.Descriptor().FullName(), name))
		}
		canWrite[name] = true
	}
	out := map[string]bool{}
	if len(mask.GetPaths()) == 0 {
		for name := range canWrite {
			if m.Has(fields.ByName(protoreflect.Name(name))) {
				out[name] = true
			}
		}
		return out, nil
	}
	for _, path := range mask.GetPaths() {
		switch {
		case path == "*":
			maps.Copy(out, canWrite)
		case canWrite[path]:
			out[path] = true
		case fields.ByName(protoreflect.Name(path)) == nil:
			return nil, apperr.Invalid("update_mask", "invalid",
				fmt.Sprintf("update_mask: %q is not a field of %s", path, m.Descriptor().Name()))
		}
	}
	return out, nil
}

// Under reports a module's invalid fields at the request field that holds the module's input: the module says title,
// the request has it at item.title. Other errors are returned as they are.
func Under(field string, err error) error {
	fields, ok := apperr.Fields(err)
	if !ok {
		return err
	}
	under := make([]error, len(fields))
	for i, f := range fields {
		moved := *f
		moved.Field = field + "." + f.Field
		under[i] = &moved
	}
	if len(under) == 1 {
		return under[0]
	}
	return errors.Join(under...)
}
