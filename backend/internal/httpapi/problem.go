package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"runtime/debug"
	"strconv"

	"github.com/Will413028/fullstack-template/backend/internal/httpapi/gen"
	"github.com/Will413028/fullstack-template/backend/internal/platform/apperr"
)

// problemType is one kind of RFC 9457 problem. type and title are fixed per kind; clients choose messages by type
// and code (docs/adr/2026-10-18-api-and-auth-baseline.md D3), so changing a slug is a breaking change.
type problemType struct {
	status int
	slug   string
	title  string
}

func (p problemType) uri() string { return "/problems/" + p.slug }

var (
	problemBadRequest           = problemType{http.StatusBadRequest, "bad-request", "Bad request"}
	problemUnauthenticated      = problemType{http.StatusUnauthorized, "unauthenticated", "Unauthenticated"}
	problemForbidden            = problemType{http.StatusForbidden, "forbidden", "Forbidden"}
	problemNotFound             = problemType{http.StatusNotFound, "not-found", "Not found"}
	problemMethodNotAllowed     = problemType{http.StatusMethodNotAllowed, "method-not-allowed", "Method not allowed"}
	problemConflict             = problemType{http.StatusConflict, "conflict", "Conflict"}
	problemPayloadTooLarge      = problemType{http.StatusRequestEntityTooLarge, "payload-too-large", "Payload too large"}
	problemUnsupportedMediaType = problemType{http.StatusUnsupportedMediaType, "unsupported-media-type", "Unsupported media type"}
	problemValidation           = problemType{http.StatusUnprocessableEntity, "validation", "Validation failed"}
	problemRateLimited          = problemType{http.StatusTooManyRequests, "rate-limited", "Too many requests"}
	problemInternal             = problemType{http.StatusInternalServerError, "internal", "Internal error"}
	problemUnavailable          = problemType{http.StatusServiceUnavailable, "unavailable", "Service unavailable"}
)

var kindProblems = map[apperr.Kind]problemType{
	apperr.NotFound:        problemNotFound,
	apperr.Conflict:        problemConflict,
	apperr.Unauthenticated: problemUnauthenticated,
	apperr.Forbidden:       problemForbidden,
	apperr.Validation:      problemValidation,
	apperr.RateLimited:     problemRateLimited,
}

// Codes for problems the HTTP layer raises itself.
const codeCrossOrigin = "cross_origin_request"

const problemContentType = "application/problem+json"

// writeProblem writes a problem of type p. detail and code are omitted when empty.
func writeProblem(w http.ResponseWriter, p problemType, detail, code string) {
	body := gen.Problem{Type: gen.ProblemType(p.uri()), Title: p.title, Status: p.status, RequestId: requestIDOf(w)}
	if detail != "" {
		body.Detail = &detail
	}
	if code != "" {
		c := gen.ProblemCode(code)
		body.Code = &c
	}
	writeProblemBody(w, p.status, body)
}

// writeValidation writes a validation problem listing each invalid input.
func writeValidation(w http.ResponseWriter, fields []gen.FieldError) {
	p := problemValidation
	writeProblemBody(w, p.status, gen.ValidationProblem{
		Type: gen.ValidationProblemType(p.uri()), Title: p.title, Status: p.status, Errors: fields, RequestId: requestIDOf(w),
	})
}

// requestIDOf returns the request id withRequestInfo put in the X-Request-ID response header, so the body and the
// header always agree; nil without one.
func requestIDOf(w http.ResponseWriter) *string {
	if id := w.Header().Get(headerRequestID); id != "" {
		return &id
	}
	return nil
}

func writeProblemBody(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", problemContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError turns err into a problem. Expected failures (*apperr.Error) and an oversized body map to their problem;
// anything else is a 500 whose error is only logged, never sent: it can hold SQL, hostnames or user data.
//
// When the response has already started, a problem would be appended to it; the response is aborted instead.
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	if responseStarted(w) {
		logger.ErrorContext(r.Context(), "error after the response started", "method", r.Method, "path", r.URL.Path,
			"error", err)
		panic(http.ErrAbortHandler)
	}
	if e, ok := apperr.As(err); ok {
		p, known := kindProblems[e.Kind]
		if !known {
			logger.ErrorContext(r.Context(), "apperr with unknown kind", "kind", int(e.Kind), "error", err)
			writeProblem(w, problemInternal, "", "")
			return
		}
		switch e.Kind {
		case apperr.Validation:
			writeValidation(w, []gen.FieldError{{Pointer: e.Pointer, Code: gen.FieldErrorCode(e.Code), Detail: e.Detail}})
			return
		case apperr.RateLimited:
			if e.RetryAfter > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(e.RetryAfter.Seconds()))))
			}
		default:
			// the other kinds map to their problem as they are
		}
		writeProblem(w, p, e.Detail, e.Code)
		return
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeProblem(w, problemPayloadTooLarge, "request body is over "+strconv.FormatInt(tooLarge.Limit, 10)+" bytes", "")
		return
	}
	logger.ErrorContext(r.Context(), "unhandled error", "method", r.Method, "path", r.URL.Path, "error", err)
	writeProblem(w, problemInternal, "", "")
}

// recoverPanics turns a panic in h into a logged 500 problem instead of a dropped connection. http.ErrAbortHandler
// is re-raised: it is the documented way for a handler to abort the response. A panic after the response started
// aborts it too: appending a problem would turn a partial 200 into a 200 with corrupt JSON.
func recoverPanics(logger *slog.Logger, h http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w := &startWriter{ResponseWriter: rw}
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(v)
			}
			logger.ErrorContext(r.Context(), "panic", "method", r.Method, "path", r.URL.Path, "panic", v,
				"stack", string(debug.Stack()))
			if w.started {
				panic(http.ErrAbortHandler)
			}
			writeProblem(w, problemInternal, "", "")
		}()
		h.ServeHTTP(w, r)
	})
}

// startWriter records whether the response has started (status or body written).
type startWriter struct {
	http.ResponseWriter
	started bool
}

func (w *startWriter) WriteHeader(code int) {
	if code >= 200 {
		w.started = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *startWriter) Write(b []byte) (int, error) {
	w.started = true
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer (Flush, deadlines).
func (w *startWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// responseStarted reports whether a startWriter in w's wrapping chain saw the response start.
func responseStarted(w http.ResponseWriter) bool {
	for {
		switch v := w.(type) {
		case *startWriter:
			return v.started
		case interface{ Unwrap() http.ResponseWriter }:
			w = v.Unwrap()
		default:
			return false
		}
	}
}
