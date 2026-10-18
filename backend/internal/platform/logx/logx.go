// Package logx builds the application logger: readable text in dev, JSON in prod.
package logx

import (
	"io"
	"log/slog"
)

// New returns a logger writing to w for mode (dev or prod).
func New(w io.Writer, mode string) *slog.Logger {
	if mode == "prod" {
		return slog.New(slog.NewJSONHandler(w, nil))
	}
	return slog.New(slog.NewTextHandler(w, nil))
}
