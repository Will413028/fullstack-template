package config

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

func lookup(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func baseEnv() map[string]string {
	return map[string]string{"DATABASE_URL": "postgres://app:pw@db:5432/app?sslmode=disable"}
}

func TestConfigDefaults(t *testing.T) {
	cfg, err := Load(lookup(baseEnv()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mode != "dev" || cfg.HTTPAddr != ":8000" || !cfg.CookieSecure {
		t.Errorf("mode=%q addr=%q secure=%v", cfg.Mode, cfg.HTTPAddr, cfg.CookieSecure)
	}
	if cfg.SessionAbsolute != 168*time.Hour || cfg.SessionIdle != 24*time.Hour {
		t.Errorf("session absolute=%v idle=%v", cfg.SessionAbsolute, cfg.SessionIdle)
	}
	if cfg.DBMaxConns != 20 || cfg.DBStatementTimeout != 5*time.Second {
		t.Errorf("db max=%d timeout=%v", cfg.DBMaxConns, cfg.DBStatementTimeout)
	}
	if cfg.AuthRateLimitPerMin != 5 || cfg.PasswordHIBPCheck || len(cfg.TrustedProxies) != 0 {
		t.Errorf("rate=%d hibp=%v proxies=%v", cfg.AuthRateLimitPerMin, cfg.PasswordHIBPCheck, cfg.TrustedProxies)
	}
}

func TestConfigRequiresDatabaseURL(t *testing.T) {
	if _, err := Load(lookup(map[string]string{})); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v, want missing DATABASE_URL", err)
	}
}

func TestConfigRejectsBadTrustedProxies(t *testing.T) {
	env := baseEnv()
	env["TRUSTED_PROXIES"] = "172.30.0.0/24,not-a-cidr"
	if _, err := Load(lookup(env)); err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXIES") {
		t.Fatalf("err = %v, want TRUSTED_PROXIES error", err)
	}

	env["TRUSTED_PROXIES"] = "172.30.0.0/24, 10.0.0.1/32"
	cfg, err := Load(lookup(env))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []netip.Prefix{netip.MustParsePrefix("172.30.0.0/24"), netip.MustParsePrefix("10.0.0.1/32")}
	if len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[0] != want[0] || cfg.TrustedProxies[1] != want[1] {
		t.Errorf("TrustedProxies = %v, want %v", cfg.TrustedProxies, want)
	}
}

func TestCookieNameFollowsSecure(t *testing.T) {
	for _, tc := range []struct {
		secure string
		want   string
	}{{"true", "__Host-session"}, {"false", "session"}} {
		env := baseEnv()
		env["COOKIE_SECURE"] = tc.secure
		cfg, err := Load(lookup(env))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got := cfg.CookieName(); got != tc.want {
			t.Errorf("COOKIE_SECURE=%s: CookieName() = %q, want %q", tc.secure, got, tc.want)
		}
	}
}

func TestConfigRejectsDriverSuffixInDatabaseURL(t *testing.T) {
	env := baseEnv()
	env["DATABASE_URL"] = "postgresql+asyncpg://app:pw@db:5432/app"
	_, err := Load(lookup(env))
	if err == nil || !strings.Contains(err.Error(), "+asyncpg") {
		t.Fatalf("err = %v, want driver suffix error", err)
	}

	env["DATABASE_URL"] = "mysql://app:pw@db/app"
	if _, err := Load(lookup(env)); err == nil {
		t.Fatal("non-postgres scheme accepted")
	}

	env["DATABASE_URL"] = "postgresql://app:pw@db:5432/app"
	if _, err := Load(lookup(env)); err != nil {
		t.Fatalf("postgresql:// rejected: %v", err)
	}
}

func TestConfigSessionIdleNotAboveAbsolute(t *testing.T) {
	env := baseEnv()
	env["SESSION_ABSOLUTE"] = "24h"
	env["SESSION_IDLE"] = "48h"
	if _, err := Load(lookup(env)); err == nil || !strings.Contains(err.Error(), "SESSION_IDLE") {
		t.Fatalf("err = %v, want SESSION_IDLE error", err)
	}

	env["SESSION_IDLE"] = "24h"
	if _, err := Load(lookup(env)); err != nil {
		t.Fatalf("idle == absolute rejected: %v", err)
	}
}

func TestConfigReportsAllProblems(t *testing.T) {
	env := map[string]string{"MODE": "staging", "DB_MAX_CONNS": "many"}
	_, err := Load(lookup(env))
	if err == nil {
		t.Fatal("Load succeeded")
	}
	for _, key := range []string{"MODE", "DB_MAX_CONNS", "DATABASE_URL"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not mention %s", err, key)
		}
	}
}
