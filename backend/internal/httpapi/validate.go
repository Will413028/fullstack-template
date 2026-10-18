package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/Will413028/fullstack-template/backend/internal/httpapi/gen"
)

// Without a validator kin-openapi accepts any string as format: uuid; RFC 9562 covers the UUIDv7 primary keys. It is
// registered globally because kin-openapi applies document-scoped formats (T.SetStringFormatValidator) to request
// bodies only, not to path and query parameters.
//
// The JSON body decoders are replaced with one that rejects data after the first value ({"a":1} trailing):
// kin-openapi's stops after the first value, and so does the generated code's json.Decoder, so the trailing data
// would otherwise be silently ignored.
func init() {
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC9562))
	for _, mediaType := range []string{"application/json", "application/merge-patch+json"} {
		openapi3filter.RegisterBodyDecoder(mediaType, decodeSingleJSON)
	}
}

// decodeSingleJSON decodes exactly one JSON value, numbers as json.Number like kin-openapi's own decoder.
func decodeSingleJSON(body io.Reader, _ http.Header, _ *openapi3.SchemaRef, _ openapi3filter.EncodingFn) (any, error) {
	dec := json.NewDecoder(body)
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, &openapi3filter.ParseError{Kind: openapi3filter.KindInvalidFormat, Cause: err}
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, &openapi3filter.ParseError{Kind: openapi3filter.KindInvalidFormat, Reason: "data after the JSON value"}
	}
	return value, nil
}

// loadSpec parses and validates an OpenAPI document.
func loadSpec(ctx context.Context, data []byte) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.Context = ctx
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return nil, err
	}
	if err := doc.Validate(ctx); err != nil {
		return nil, err
	}
	return doc, nil
}

// validateRequests checks every request against spec before next sees it, so length, format and enum rules live
// only in the contract (plan I1). It is mounted on /api/ only; spec's servers entry (/api) is the path prefix.
// Failures become problems: an undefined route 404, a body over the limit 413, a missing session where the operation
// declares security 401, a Content-Type the operation does not accept 415, a body that is not valid JSON 400, and
// anything else 422 listing each invalid input.
func validateRequests(spec *openapi3.T, logger *slog.Logger, next http.Handler) http.Handler {
	return nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		// requireSession enforces the contract's security; authenticate has put the session's user in the context.
		Options: openapi3filter.Options{MultiError: true, AuthenticationFunc: requireSession},
		// The only server is the relative /api, which the router matches as a path prefix without a Host check.
		SilenceServersWarning: true,
		ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, r *http.Request,
			opts nethttpmiddleware.ErrorHandlerOpts) {
			writeValidationError(w, r, logger, err, opts.MatchedRoute)
		},
	})(next)
}

func writeValidationError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error,
	matched *nethttpmiddleware.ErrorHandlerOptsMatchedRoute) {
	if matched == nil {
		if errors.Is(err, routers.ErrMethodNotAllowed) {
			writeProblem(w, problemMethodNotAllowed, "", "")
			return
		}
		writeProblem(w, problemNotFound, "", "")
		return
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, r, logger, tooLarge)
		return
	}
	// Security is checked before parameters and body, and its failure wins over theirs: an anonymous caller learns
	// nothing about the request it would have to send.
	var secErr *openapi3filter.SecurityRequirementsError
	if errors.As(err, &secErr) {
		writeProblem(w, problemUnauthenticated, "this operation needs a signed-in session", "")
		return
	}
	if accepted, unsupported := unsupportedMediaType(r, matched.Route); unsupported {
		writeProblem(w, problemUnsupportedMediaType, "accepted Content-Type: "+strings.Join(accepted, ", "), "")
		return
	}
	var c collector
	c.walk(err, "")
	if c.readErr != nil {
		writeError(w, r, logger, c.readErr)
		return
	}
	if c.malformed {
		writeProblem(w, problemBadRequest, "request body is not valid JSON", "")
		return
	}
	if len(c.fields) == 0 {
		// Every validation error kin-openapi returns is handled above; reaching here means a new kind of error.
		writeError(w, r, logger, err)
		return
	}
	writeValidation(w, c.fields)
}

// unsupportedMediaType reports whether the request has a body whose Content-Type the operation does not declare,
// and the media types it accepts. It repeats kin-openapi's own lookup (Content.Get) rather than matching its error
// text.
func unsupportedMediaType(r *http.Request, route *routers.Route) ([]string, bool) {
	if route == nil || route.Operation == nil || route.Operation.RequestBody == nil {
		return nil, false
	}
	body := route.Operation.RequestBody.Value
	if body == nil || len(body.Content) == 0 || r.ContentLength == 0 {
		return nil, false
	}
	if body.Content.Get(r.Header.Get("Content-Type")) != nil {
		return nil, false
	}
	accepted := make([]string, 0, len(body.Content))
	for mediaType := range body.Content {
		accepted = append(accepted, mediaType)
	}
	slices.Sort(accepted)
	return accepted, true
}

// collector flattens kin-openapi's errors into one FieldError per invalid input.
type collector struct {
	fields    []gen.FieldError
	malformed bool
	// readErr is a failure to read the body (client gone, read timeout): not the client's input, so not a 422.
	readErr error
	// unknownSeen records objects whose unknown properties were already listed: kin-openapi reports each unknown
	// property as a separate error on the object, without saying which property.
	unknownSeen map[string]bool
}

func (c *collector) add(pointer, code, detail string) {
	c.fields = append(c.fields, gen.FieldError{Pointer: pointer, Code: code, Detail: detail})
}

func (c *collector) walk(err error, base string) {
	// A type switch, not errors.As: kin-openapi nests these concrete types (MultiError of RequestError of
	// MultiError of SchemaError), and errors.As would skip a level, for example match a RequestError inside a
	// MultiError, or a SchemaError inside a RequestError and lose the parameter name.
	switch e := err.(type) { //nolint:errorlint // see above
	case openapi3.MultiError:
		for _, inner := range e {
			c.walk(inner, base)
		}
	case *openapi3filter.RequestError:
		c.walkRequestError(e)
	case *openapi3.SchemaError:
		c.walkSchemaError(e, base)
	case *openapi3filter.ParseError:
		c.add(base, "invalid_type", "value cannot be parsed as the declared type")
	default:
		c.add(base, "invalid", "invalid value")
	}
}

func (c *collector) walkRequestError(e *openapi3filter.RequestError) {
	base := ""
	if e.Parameter != nil {
		base = "/" + escapePointerToken(e.Parameter.Name)
	}
	switch {
	case errors.Is(e.Err, openapi3filter.ErrInvalidRequired):
		c.add(base, "required", "value is required")
	case errors.Is(e.Err, openapi3filter.ErrInvalidEmptyValue):
		// An empty value (?limit=) is present, so not "required"; the parameter may still be optional.
		c.add(base, "invalid", "value must not be empty")
	case e.RequestBody != nil && isParseError(e.Err):
		// The body is not valid JSON (or not decodable as its media type): a malformed request, not invalid input.
		c.malformed = true
	case e.Err == nil:
		c.add(base, "invalid", e.Reason)
	case e.RequestBody != nil && !isValidationError(e.Err):
		c.readErr = e.Err
	default:
		c.walk(e.Err, base)
	}
}

func isParseError(err error) bool {
	var p *openapi3filter.ParseError
	return errors.As(err, &p)
}

// isValidationError reports whether a request body error is about the input: a schema violation (single or
// multiple). Anything else on the body, such as an I/O error from reading it, is not.
func isValidationError(err error) bool {
	var schemaErr *openapi3.SchemaError
	var multi openapi3.MultiError
	return errors.As(err, &schemaErr) || errors.As(err, &multi)
}

// schemaCodes maps a failed JSON Schema keyword to the stable code clients translate. Keywords not listed become
// "invalid".
var schemaCodes = map[string]string{
	"required":         "required",
	"minLength":        "too_short",
	"minItems":         "too_short",
	"maxLength":        "too_long",
	"maxItems":         "too_long",
	"pattern":          "pattern",
	"enum":             "enum",
	"format":           "format",
	"minimum":          "out_of_range",
	"maximum":          "out_of_range",
	"exclusiveMinimum": "out_of_range",
	"exclusiveMaximum": "out_of_range",
	"type":             "invalid_type",
	"nullable":         "null_not_allowed",
	// kin-openapi reports a property that additionalProperties: false forbids under the "properties" keyword.
	"properties": "additional_property",
}

func (c *collector) walkSchemaError(e *openapi3.SchemaError, base string) {
	pointer := base
	for _, token := range e.JSONPointer() {
		pointer += "/" + escapePointerToken(token)
	}
	code, ok := schemaCodes[e.SchemaField]
	if !ok {
		code = "invalid"
	}
	if code == "additional_property" {
		c.addUnknownProperties(e, pointer)
		return
	}
	// Reason never contains the input value (kin-openapi's SchemaError contract), so it is safe to echo.
	c.add(pointer, code, e.Reason)
}

// addUnknownProperties lists each property of the object that its schema does not declare, pointing at the
// property itself.
func (c *collector) addUnknownProperties(e *openapi3.SchemaError, objectPointer string) {
	if c.unknownSeen[objectPointer] {
		return
	}
	if c.unknownSeen == nil {
		c.unknownSeen = map[string]bool{}
	}
	c.unknownSeen[objectPointer] = true
	obj, ok := e.Value.(map[string]any)
	if !ok || e.Schema == nil {
		c.add(objectPointer, "additional_property", e.Reason)
		return
	}
	var unknown []string
	for k := range obj {
		if _, declared := e.Schema.Properties[k]; !declared {
			unknown = append(unknown, k)
		}
	}
	slices.Sort(unknown)
	for _, k := range unknown {
		c.add(objectPointer+"/"+escapePointerToken(k), "additional_property", "property is not allowed")
	}
}

// escapePointerToken escapes one JSON Pointer reference token (RFC 6901 section 3).
func escapePointerToken(s string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(s)
}
