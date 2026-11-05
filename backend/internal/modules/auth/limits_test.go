package auth

import (
	"strings"
	"testing"

	"github.com/Will413028/template-go-nextjs/backend/internal/platform/apperr"
)

// The minimum counts the password after NFKC, the form that is hashed; the maximum counts the raw input.
func TestCheckNewAccountPasswordLengthAfterNFKC(t *testing.T) {
	// 8 times e plus a combining acute: 16 code points as sent and 8 after NFKC (é)
	assertPasswordReason(t, strings.Repeat("e\u0301", 8), "too_short")
	// 15 of them: 30 code points as sent, 15 after NFKC, the minimum
	assertPasswordReason(t, strings.Repeat("e\u0301", 15), "")
	// 65 of them: 130 code points as sent, over the maximum although NFKC leaves 65
	assertPasswordReason(t, strings.Repeat("e\u0301", 65), "too_long")
}

func assertPasswordReason(t *testing.T, password, want string) {
	t.Helper()
	got := ""
	if fields, ok := apperr.Fields(checkNewAccount("alice", password)); ok {
		for _, f := range fields {
			if f.Field == "password" {
				got = f.Code
			}
		}
	}
	if got != want {
		t.Errorf("checkNewAccount(%d code points) password reason = %q, want %q", len([]rune(password)), got, want)
	}
}
