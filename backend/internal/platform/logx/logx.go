// Package logx builds the application logger: readable text in dev, JSON in prod. Records logged with a context
// that carries a request id get a request_id attribute, so every line a request logs can be matched with it.
package logx

import (
	"context"
	"io"
	"log/slog"
)

// New returns a logger writing to w for mode (dev or prod).
func New(w io.Writer, mode string) *slog.Logger {
	var h slog.Handler = slog.NewTextHandler(w, nil)
	if mode == "prod" {
		h = slog.NewJSONHandler(w, nil)
	}
	return slog.New(WithContext(h))
}

type requestIDKey struct{}

// WithRequestID returns ctx carrying the request id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the request id in ctx, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// WithContext wraps h so a record logged with a context carrying a request id gets a request_id attribute.
func WithContext(h slog.Handler) slog.Handler { return contextHandler{h} }

type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}
