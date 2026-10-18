// Package config reads settings from environment variables and validates them once at startup.
//
// A missing required value or a malformed one never falls back to a default: every problem is collected
// and Load reports them together, so the process fails at startup. Variables and defaults match `.env-example`.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Config holds the settings serve needs.
type Config struct {
	AppName  string
	Mode     string // dev or prod
	HTTPAddr string

	DatabaseURL        string
	DBMaxConns         int32
	DBStatementTimeout time.Duration

	SessionAbsolute time.Duration
	SessionIdle     time.Duration
	// CookieSecure is false only for local development over http.
	CookieSecure bool

	// TrustedProxies are the peers whose X-Forwarded-For and X-Request-ID are trusted; empty trusts none.
	TrustedProxies      []netip.Prefix
	AuthRateLimitPerMin int
	PasswordHIBPCheck   bool
}

// CookieName is the session cookie name. The `__Host-` prefix requires Secure, so a non-Secure cookie drops it.
func (c Config) CookieName() string {
	if c.CookieSecure {
		return "__Host-session"
	}
	return "session"
}

// Load reads the settings through lookup (normally os.LookupEnv).
func Load(lookup func(string) (string, bool)) (Config, error) {
	e := &env{lookup: lookup}
	cfg := Config{
		AppName:  e.str("APP_NAME", "fullstack-template"),
		Mode:     e.str("MODE", "dev"),
		HTTPAddr: e.str("HTTP_ADDR", ":8000"),

		DatabaseURL:        e.databaseURL("DATABASE_URL"),
		DBMaxConns:         parse(e, "DB_MAX_CONNS", int32(20), parseInt32),
		DBStatementTimeout: parse(e, "DB_STATEMENT_TIMEOUT", 5*time.Second, time.ParseDuration),

		SessionAbsolute: parse(e, "SESSION_ABSOLUTE", 168*time.Hour, time.ParseDuration),
		SessionIdle:     parse(e, "SESSION_IDLE", 24*time.Hour, time.ParseDuration),
		CookieSecure:    parse(e, "COOKIE_SECURE", true, strconv.ParseBool),

		TrustedProxies:      parse(e, "TRUSTED_PROXIES", []netip.Prefix(nil), parsePrefixes),
		AuthRateLimitPerMin: parse(e, "AUTH_RATE_LIMIT_PER_MIN", 5, strconv.Atoi),
		PasswordHIBPCheck:   parse(e, "PASSWORD_HIBP_CHECK", false, strconv.ParseBool),
	}
	if cfg.Mode != "dev" && cfg.Mode != "prod" {
		e.fail("MODE must be dev or prod, got %q", cfg.Mode)
	}
	if cfg.DBMaxConns < 1 {
		e.fail("DB_MAX_CONNS must be positive")
	}
	if cfg.AuthRateLimitPerMin < 1 {
		e.fail("AUTH_RATE_LIMIT_PER_MIN must be positive")
	}
	if cfg.SessionIdle <= 0 || cfg.SessionAbsolute <= 0 {
		e.fail("SESSION_IDLE and SESSION_ABSOLUTE must be positive")
	}
	if cfg.SessionIdle > cfg.SessionAbsolute {
		e.fail("SESSION_IDLE (%s) must not exceed SESSION_ABSOLUTE (%s)", cfg.SessionIdle, cfg.SessionAbsolute)
	}
	return cfg, errors.Join(e.problems...)
}

type env struct {
	lookup   func(string) (string, bool)
	problems []error
}

func (e *env) fail(format string, args ...any) {
	e.problems = append(e.problems, fmt.Errorf(format, args...))
}

func (e *env) get(key string) (string, bool) {
	v, ok := e.lookup(key)
	return v, ok && v != ""
}

func (e *env) str(key, def string) string {
	if v, ok := e.get(key); ok {
		return v
	}
	return def
}

// databaseURL accepts only the postgres:// and postgresql:// schemes pgx understands. A driver suffix such as the
// old `postgresql+asyncpg://` fails instead of being rewritten, so a stale .env never looks like it works.
func (e *env) databaseURL(key string) string {
	raw, ok := e.get(key)
	if !ok {
		e.fail("missing required environment variable %s", key)
		return ""
	}
	scheme, _, _ := strings.Cut(raw, "://")
	if base, driver, found := strings.Cut(scheme, "+"); found {
		e.fail("%s must not carry the driver suffix +%s; use %s://", key, driver, base)
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
		e.fail("%s must be a postgres:// or postgresql:// connection string", key)
		return ""
	}
	return raw
}

func parse[T any](e *env, key string, def T, fn func(string) (T, error)) T {
	raw, ok := e.get(key)
	if !ok {
		return def
	}
	v, err := fn(raw)
	if err != nil {
		e.fail("environment variable %s is malformed: %w", key, err)
		return def
	}
	return v
}

func parseInt32(s string) (int32, error) {
	v, err := strconv.ParseInt(s, 10, 32)
	return int32(v), err
}

// parsePrefixes parses a comma-separated CIDR list. A single address must be written as /32 or /128, so a
// network is never mistyped as one address.
func parsePrefixes(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for part := range strings.SplitSeq(s, ",") {
		p, err := netip.ParsePrefix(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		out = append(out, p.Masked())
	}
	return out, nil
}
