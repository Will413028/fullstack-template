package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"runtime"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

// normalizePassword applies NFKC before a password is checked against the list or hashed, so the same password
// typed with full-width or half-width characters is the same password (NIST SP 800-63B-4 asks for normalization).
// The contract's length limits count the raw input.
func normalizePassword(p string) string { return norm.NFKC.String(p) }

// PasswordHasher hashes passwords and checks them against stored hashes.
type PasswordHasher interface {
	Hash(ctx context.Context, password string) (string, error)
	// Verify reports whether password matches encoded. A hash that does not parse is a mismatch.
	Verify(ctx context.Context, password, encoded string) (bool, error)
}

// Argon2Params are the argon2id cost parameters.
type Argon2Params struct {
	MemoryKiB uint32
	Time      uint32
	Threads   uint8
}

// DefaultArgon2Params is OWASP Password Storage's argon2id minimum: 19 MiB, 2 iterations, 1 thread.
var DefaultArgon2Params = Argon2Params{MemoryKiB: 19 * 1024, Time: 2, Threads: 1}

const (
	saltLen = 16
	keyLen  = 32
)

// Argon2Hasher stores argon2id hashes as PHC strings. Each hash holds MemoryKiB for its duration, so at most
// maxConcurrent run at once; the rest wait, and give up when their context ends.
type Argon2Hasher struct {
	params Argon2Params
	sem    chan struct{}
	// key is argon2.IDKey; tests replace it.
	key func(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte
}

// NewArgon2Hasher returns a hasher running at most maxConcurrent hashes at once; 0 means runtime.NumCPU().
func NewArgon2Hasher(params Argon2Params, maxConcurrent int) *Argon2Hasher {
	if maxConcurrent <= 0 {
		maxConcurrent = runtime.NumCPU()
	}
	return &Argon2Hasher{params: params, sem: make(chan struct{}, maxConcurrent), key: argon2.IDKey}
}

func (h *Argon2Hasher) derive(ctx context.Context, password, salt []byte, p Argon2Params) ([]byte, error) {
	select {
	case h.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-h.sem }()
	return h.key(password, salt, p.Time, p.MemoryKiB, p.Threads, keyLen), nil
}

// Hash returns $argon2id$v=19$m=<KiB>,t=<time>,p=<threads>$<salt>$<key>, base64 without padding.
func (h *Argon2Hasher) Hash(ctx context.Context, password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := h.derive(ctx, []byte(password), salt, h.params)
	if err != nil {
		return "", err
	}
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, h.params.MemoryKiB, h.params.Time,
		h.params.Threads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// Verify recomputes the key with the parameters stored in encoded, so hashes made with older parameters still
// verify.
func (h *Argon2Hasher) Verify(ctx context.Context, password, encoded string) (bool, error) {
	p, salt, want, ok := parsePHC(encoded)
	if !ok {
		return false, nil
	}
	got, err := h.derive(ctx, []byte(password), salt, p)
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// parsePHC reads an argon2id PHC string; ok is false for anything else.
func parsePHC(encoded string) (p Argon2Params, salt, key []byte, ok bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return p, nil, nil, false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.MemoryKiB, &p.Time, &p.Threads); err != nil {
		return p, nil, nil, false
	}
	if p.MemoryKiB == 0 || p.Time == 0 || p.Threads == 0 {
		return p, nil, nil, false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return p, nil, nil, false
	}
	key, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return p, nil, nil, false
	}
	return p, salt, key, true
}
