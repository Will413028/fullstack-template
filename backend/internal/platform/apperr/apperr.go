// Package apperr is how services report expected failures: a kind that decides the HTTP status and problem type,
// and a stable code clients translate. Services return *Error; internal/httpapi turns it into an RFC 9457 problem
// in one place (problem.go). Any other error is unexpected and becomes a 500 whose details stay in the log.
package apperr

import (
	"errors"
	"time"
)

// Kind is the category of failure; each maps to one problem type.
type Kind int

const (
	NotFound Kind = iota + 1
	Conflict
	Unauthenticated
	Forbidden
	Validation
	RateLimited
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
	// Pointer is the JSON Pointer to the invalid input, for Validation.
	Pointer string
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

// Invalid returns a Validation error for the input at pointer, for rules the contract cannot express (for example
// a password on the common-password list).
func Invalid(pointer, code, detail string) *Error {
	return &Error{Kind: Validation, Code: code, Detail: detail, Pointer: pointer}
}

// As returns the *Error in err's chain, if any.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
