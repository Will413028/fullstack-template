// Package hibp checks passwords against Have I Been Pwned's Pwned Passwords range API with k-anonymity: only the
// first 5 hex characters of the password's SHA-1 leave the process, and responses are padded (Add-Padding) so their
// size does not hint at the prefix. Enabled with PASSWORD_HIBP_CHECK=true.
package hibp

import (
	"bufio"
	"context"
	"crypto/sha1" //nolint:gosec // the range API is keyed by SHA-1; nothing here relies on its collision resistance
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is the public range API.
const DefaultBaseURL = "https://api.pwnedpasswords.com"

// Client queries the range API.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a client for the public API with a 3-second timeout, short enough not to hold up a sign-up.
func New() *Client {
	return &Client{BaseURL: DefaultBaseURL, HTTP: &http.Client{Timeout: 3 * time.Second}}
}

// Breached reports whether password appears in Pwned Passwords. Padding entries have a count of 0 and do not count.
func (c *Client) Breached(ctx context.Context, password string) (bool, error) {
	sum := sha1.Sum([]byte(password)) //nolint:gosec // see the import
	digest := strings.ToUpper(hex.EncodeToString(sum[:]))
	prefix, suffix := digest[:5], digest[5:]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/range/"+prefix, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Add-Padding", "true")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false, fmt.Errorf("hibp: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("hibp: %s", resp.Status)
	}
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		hash, count, ok := strings.Cut(strings.TrimSpace(sc.Text()), ":")
		if ok && strings.EqualFold(hash, suffix) {
			return count != "0", nil
		}
	}
	if err := sc.Err(); err != nil {
		return false, fmt.Errorf("hibp: read response: %w", err)
	}
	return false, nil
}
