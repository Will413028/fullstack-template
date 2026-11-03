package rpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	commonv1 "github.com/Will413028/template-go-nextjs/backend/internal/gen/app/common/v1"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/apperr"
)

// Errors are a Connect code plus google.rpc details (docs/rules/api-design.md): ErrorInfo with a reason from
// app.common.v1.ErrorReason when the client needs to tell failures apart, BadRequest for invalid fields, RetryInfo
// when retrying later helps. The message is for developers; clients translate the code and the reasons.

// ErrorDomain is ErrorInfo's domain: the proto root, which make rename keeps (decision D5).
const ErrorDomain = "app"

// kindCodes maps each apperr kind to its Connect code.
var kindCodes = map[apperr.Kind]connect.Code{
	apperr.Validation:         connect.CodeInvalidArgument,
	apperr.NotFound:           connect.CodeNotFound,
	apperr.AlreadyExists:      connect.CodeAlreadyExists,
	apperr.FailedPrecondition: connect.CodeFailedPrecondition,
	apperr.Unauthenticated:    connect.CodeUnauthenticated,
	apperr.Forbidden:          connect.CodePermissionDenied,
	apperr.RateLimited:        connect.CodeResourceExhausted,
}

// Converted is an error as the client sees it.
type Converted struct {
	Err *connect.Error
	// Internal is set when err was not an expected failure: the client gets only the code, and the caller logs err
	// at error level (its text can hold SQL, hostnames or user data).
	Internal bool
	// Undeclared lists apperr codes that ErrorReason or FieldViolationReason does not declare; they were left out of
	// the details, and the caller logs them (a test keeps every code the modules return declared).
	Undeclared []string
}

// Convert turns err into a Connect error. A *connect.Error passes through unchanged unless its code is internal or
// unknown: its message could carry what the client must not see (a wrapped database error, a downstream call's
// text), so it is internal like any unexpected error. An *apperr.Error gets its kind's code and details; anything
// else is internal.
func Convert(err error) Converted {
	var ce *connect.Error
	if errors.As(err, &ce) {
		if ce.Code() == connect.CodeInternal || ce.Code() == connect.CodeUnknown {
			return internal()
		}
		return Converted{Err: ce}
	}
	if fields, ok := apperr.Fields(err); ok {
		return invalidFields(fields)
	}
	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
		// what connect-go answers for a body over its reader's limit, without RetryInfo (which marks the rate limit)
		return Converted{Err: connect.NewError(connect.CodeResourceExhausted, errors.New("request body is too large"))}
	}
	e, ok := apperr.As(err)
	if !ok || e.Kind == apperr.Validation { // a Validation that Fields refused is joined with an unexpected error
		return internal()
	}
	code, known := kindCodes[e.Kind]
	if !known {
		return internal()
	}
	out := Converted{Err: connect.NewError(code, errors.New(e.Detail))}
	if e.Code != "" {
		if reason, declared := errorReason(e.Code); declared {
			addDetail(out.Err, &errdetails.ErrorInfo{Reason: reason, Domain: ErrorDomain})
		} else {
			out.Undeclared = append(out.Undeclared, e.Code)
		}
	}
	if e.Kind == apperr.RateLimited && e.RetryAfter > 0 {
		addDetail(out.Err, &errdetails.RetryInfo{RetryDelay: durationpb.New(e.RetryAfter)})
	}
	return out
}

// invalidFields is invalid_argument with one BadRequest violation per field, in the order the service reported them.
func invalidFields(fields []*apperr.Error) Converted {
	details := make([]string, 0, len(fields))
	br := &errdetails.BadRequest{}
	var undeclared []string
	for _, f := range fields {
		reason, declared := fieldReason(f.Code)
		if !declared {
			undeclared = append(undeclared, f.Code)
		}
		br.FieldViolations = append(br.FieldViolations, &errdetails.BadRequest_FieldViolation{
			Field: f.Field, Reason: reason, Description: f.Detail,
		})
		details = append(details, f.Detail)
	}
	out := Converted{Err: connect.NewError(connect.CodeInvalidArgument, errors.New(strings.Join(details, "; "))),
		Undeclared: undeclared}
	addDetail(out.Err, br)
	return out
}

func internal() Converted {
	return Converted{Err: errInternal(), Internal: true}
}

// errInternal is what the client sees of an unexpected error: the code, and no detail.
func errInternal() *connect.Error {
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}

// errorReason is the ErrorReason value for an apperr code (account_taken -> ACCOUNT_TAKEN), false when the proto
// does not declare it.
func errorReason(code string) (string, bool) {
	name := strings.ToUpper(code)
	v, ok := commonv1.ErrorReason_value[name]
	return name, ok && v != int32(commonv1.ErrorReason_ERROR_REASON_UNSPECIFIED)
}

// fieldReason is the FieldViolationReason value for an apperr field code (too_long -> TOO_LONG); an undeclared code
// becomes INVALID, so the client still gets a reason it knows.
func fieldReason(code string) (string, bool) {
	name := strings.ToUpper(code)
	v, ok := commonv1.FieldViolationReason_value[name]
	if !ok || v == int32(commonv1.FieldViolationReason_FIELD_VIOLATION_REASON_UNSPECIFIED) {
		return commonv1.FieldViolationReason_INVALID.String(), false
	}
	return name, true
}

func addDetail(ce *connect.Error, m proto.Message) {
	d, err := connect.NewErrorDetail(m)
	if err != nil {
		panic(fmt.Sprintf("rpc: error detail %T: %v", m, err)) // only a message type the registry lacks fails
	}
	ce.AddDetail(d)
}

// LogConverted logs what the client does not see: an internal error's cause, and codes the proto does not declare.
func LogConverted(ctx context.Context, logger *slog.Logger, method, procedure string, err error, c Converted) {
	if c.Internal {
		logger.ErrorContext(ctx, "internal error", "method", method, "procedure", procedure, "error", err)
	}
	if len(c.Undeclared) > 0 {
		logger.ErrorContext(ctx, "error code not declared in app.common.v1", "method", method, "procedure", procedure,
			"codes", c.Undeclared, "error", err)
	}
}

var errorWriter = connect.NewErrorWriter()

// WriteError writes ce as a Connect error, for requests the net/http layer rejects before the Connect handler runs.
func WriteError(w http.ResponseWriter, r *http.Request, ce *connect.Error) {
	_ = errorWriter.Write(w, r, ce)
}
