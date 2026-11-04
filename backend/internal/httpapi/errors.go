package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/Will413028/template-go-nextjs/backend/internal/httpapi/rpc"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/apperr"
)

// Codes for errors the HTTP layer raises itself.
const codeCrossOrigin = "cross_origin_request"

var errCrossOrigin = apperr.New(apperr.Forbidden, codeCrossOrigin, "cross-origin request rejected")

// errLogged is an error its caller has already logged: writeError sends an internal error without logging it again.
var errLogged = errors.New("internal error, already logged")

// writeError is the net/http layer's only way to send an error: a Connect error (rpc.Convert), for requests it
// rejects before the Connect handler runs and for paths that are not a procedure. Expected failures (*apperr.Error)
// map to their code; anything else is internal, and its error is only logged, never sent: it can hold SQL,
// hostnames or user data.
//
// When the response has already started, an error would be appended to it; the response is aborted instead.
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	if responseStarted(w) {
		logger.ErrorContext(r.Context(), "error after the response started", "method", r.Method, "path", r.URL.Path,
			"error", err)
		panic(http.ErrAbortHandler)
	}
	c := rpc.Convert(err)
	if !errors.Is(err, errLogged) {
		rpc.LogConverted(r.Context(), logger, r.Method, strings.TrimPrefix(r.URL.Path, "/api"), err, c)
	}
	rpc.WriteError(w, r, c.Err)
}

// recoverPanics turns a panic in h into a logged internal error (writeError) instead of a dropped connection. http.ErrAbortHandler
// is re-raised: it is the documented way for a handler to abort the response. A panic after the response started
// aborts it too: appending an error would turn a partial 200 into a 200 with corrupt JSON.
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
			writeError(w, r, logger, errLogged)
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
