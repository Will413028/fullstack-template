// Package apperr is how services report expected failures: a kind that decides the response's status, and a stable
// code clients translate. Services return *Error; internal/httpapi/rpc turns it into a Connect code with error
// details. Any other error is unexpected and
// becomes an internal error whose details stay in the log.
package apperr

import (
	"errors"
	"time"
)

// Kind is the category of failure; each maps to one Connect code. The two conflicts follow
// gRPC's status-code guidance: AlreadyExists when the thing to create exists, FailedPrecondition when the system is
// not in the state the call requires (retrying will not help until that state changes).
type Kind int

const (
	NotFound Kind = iota + 1
	AlreadyExists
	Unauthenticated
	Forbidden
	Validation
	RateLimited
	FailedPrecondition
)

// Error is an expected failure.
type Error struct {
	Kind Kind
	// Code is the stable machine-readable code clients translate, for example "account_taken". Changing it is a
	// breaking change.
	Code string
	// Detail explains the failure to developers. It is sent to the client, so it must not contain secrets or
	// internal state, and clients must not parse it.
	Detail string
	// Field is the invalid input, for Validation: a field name as the module sees it (password, title), which the
	// HTTP layer maps to the request's field path (internal/httpapi/rpc.Under).
	Field string
	// RetryAfter is how long the client should wait, for RateLimited.
	RetryAfter time.Duration
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return e.Code
	}
	return e.Code + ": " + e.Detail
}

// New returns an Error of the given kind.
func New(kind Kind, code, detail string) *Error {
	return &Error{Kind: kind, Code: code, Detail: detail}
}

// Invalid returns a Validation error for one invalid input field. A service that finds several invalid fields
// returns them together with errors.Join, so the client sees every one at once (Fields).
func Invalid(field, code, detail string) *Error {
	return &Error{Kind: Validation, Code: code, Detail: detail, Field: field}
}

// Fields returns the invalid fields err reports: a Validation error, or errors.Join of Validation errors, either
// possibly wrapped (fmt.Errorf with %w). ok is false for anything else, including a join that mixes in another error.
func Fields(err error) ([]*Error, bool) {
	for err != nil {
		if joined, isJoin := err.(interface{ Unwrap() []error }); isJoin {
			return validations(joined.Unwrap())
		}
		next := errors.Unwrap(err)
		if next == nil { // the end of the chain; an *Error wraps nothing
			return validations([]error{err})
		}
		err = next
	}
	return nil, false
}

// validations returns errs as Validation errors; false unless every one is.
func validations(errs []error) ([]*Error, bool) {
	fields := make([]*Error, 0, len(errs))
	for _, member := range errs {
		e, ok := As(member)
		if !ok || e.Kind != Validation {
			return nil, false
		}
		fields = append(fields, e)
	}
	return fields, len(fields) > 0
}

// As returns the *Error in err's chain, if any.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
