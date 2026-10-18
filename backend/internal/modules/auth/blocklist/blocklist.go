// Package blocklist holds the common and breached passwords new passwords are checked against (NIST SP 800-63B-4,
// inventory B20).
//
// common-passwords.txt is SecLists Passwords/Common-Credentials/Pwdb_top-1000000.txt at commit
// 12274c98fdebe98c7a7284914436a472ed469aed (sha256 e9a88f67aafe65496682dc374559ee714e978bee50314767494c3e37a18c9fc8,
// MIT, see LICENSE-SecLists), keeping only the distinct lines of at least 15 code points: shorter ones can never be
// a valid password here. To refresh it, download the file at a new commit, check its hash, and keep the lines
// with len(line) >= 15 in Python (code points), in their original order.
package blocklist

import (
	_ "embed"
	"strings"
	"sync"

	"golang.org/x/text/unicode/norm"
)

//go:embed common-passwords.txt
var commonPasswords string

var load = sync.OnceValue(func() map[string]struct{} {
	set := map[string]struct{}{}
	for line := range strings.SplitSeq(commonPasswords, "\n") {
		if line != "" {
			set[norm.NFKC.String(line)] = struct{}{}
		}
	}
	return set
})

// Contains reports whether the NFKC-normalized password is on the list. The match is exact: the list already
// holds the common case variants.
func Contains(normalized string) bool {
	_, ok := load()[normalized]
	return ok
}

// Len is the number of distinct entries.
func Len() int { return len(load()) }
