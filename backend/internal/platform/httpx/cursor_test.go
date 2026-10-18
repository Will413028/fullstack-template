package httpx

import (
	"encoding/base64"
	"testing"

	"github.com/google/uuid"

	"github.com/Will413028/fullstack-template/backend/internal/platform/apperr"
)

func TestCursorRoundTrip(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	got, err := DecodeCursor(EncodeCursor(id))
	if err != nil || got != id {
		t.Fatalf("DecodeCursor(EncodeCursor(%s)) = %s, %v", id, got, err)
	}
}

func TestCursorTamperedIsInvalid(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	valid := EncodeCursor(id)
	for name, cursor := range map[string]string{
		"not base64url":  valid[:10] + "*" + valid[11:],
		"padded base64":  base64.URLEncoding.EncodeToString(id[:]),
		"one byte short": base64.RawURLEncoding.EncodeToString(id[:15]),
		"one byte long":  base64.RawURLEncoding.EncodeToString(append(id[:], 0)),
		"empty":          "",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeCursor(cursor)
			e, ok := apperr.As(err)
			if !ok || e.Kind != apperr.Validation || e.Pointer != "/cursor" || e.Code != "invalid" {
				t.Fatalf("DecodeCursor(%q) err = %v, want validation on /cursor with code invalid", cursor, err)
			}
		})
	}
}
