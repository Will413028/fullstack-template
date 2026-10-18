package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Will413028/fullstack-template/backend/internal/httpapi/gen"
)

// The generated code binds parameters and decodes bodies after the validator has passed them, but its parsers are
// stricter than kin-openapi's: kin-openapi reads integers with base prefixes (limit=0x10) and accepts 5.0 as an
// integer, Go's strconv and encoding/json do not. Those requests are still invalid input, so they get the same 422
// shape as validator errors, with a fixed detail: the parsers' messages echo the input and Go type names.

// writeParamError handles a parameter the generated code could not bind.
func writeParamError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var (
		format   *gen.InvalidParamFormatError
		unmarsh  *gen.UnmarshalingParamError
		required *gen.RequiredParamError
		tooMany  *gen.TooManyValuesForParamError
	)
	switch {
	case errors.As(err, &format):
		writeParamField(w, format.ParamName, "invalid_type", "value cannot be parsed as the declared type")
	case errors.As(err, &unmarsh):
		writeParamField(w, unmarsh.ParamName, "invalid_type", "value cannot be parsed as the declared type")
	case errors.As(err, &required):
		writeParamField(w, required.ParamName, "required", "value is required")
	case errors.As(err, &tooMany):
		writeParamField(w, tooMany.ParamName, "invalid", "parameter is given more than once")
	default:
		writeError(w, r, logger, err)
	}
}

func writeParamField(w http.ResponseWriter, name, code, detail string) {
	writeValidation(w, []gen.FieldError{{Pointer: "/" + escapePointerToken(name), Code: gen.FieldErrorCode(code), Detail: detail}})
}

// writeBodyDecodeError handles a body the generated code could not decode into its request type.
func writeBodyDecodeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		writeError(w, r, logger, err)
	case errors.As(err, &typeErr):
		// Field is the path of JSON keys from the root, separated by dots. On go1.27.2 the tokens keep JSON Pointer
		// escapes (key "a/b" gives "a~1b"), so they are used as they are; the contract's property names are
		// snake_case, where escaping never applies.
		pointer := ""
		if typeErr.Field != "" {
			pointer = "/" + strings.ReplaceAll(typeErr.Field, ".", "/")
		}
		writeValidation(w, []gen.FieldError{{Pointer: pointer, Code: "invalid_type", Detail: "value cannot be parsed as the declared type"}})
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		writeProblem(w, problemBadRequest, "request body is not valid JSON", "")
	default:
		writeError(w, r, logger, err)
	}
}
