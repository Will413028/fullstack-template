package config

import (
	"net/netip"
	"os"
	"path/filepath"
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
	return map[string]string{
		"DATABASE_URL": "postgres://app@db:5432/app?sslmode=disable", "DATABASE_PASSWORD": "pw",
		"DATABASE_PLAINTEXT": "same-host",
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg, err := Load(lookup(baseEnv()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mode != "prod" || cfg.HTTPAddr != ":8000" || !cfg.CookieSecure {
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
	env["DATABASE_URL"] = "postgresql+asyncpg://app@db:5432/app"
	_, err := Load(lookup(env))
	if err == nil || !strings.Contains(err.Error(), "+asyncpg") {
		t.Fatalf("err = %v, want driver suffix error", err)
	}

	env["DATABASE_URL"] = "mysql://app@db/app"
	if _, err := Load(lookup(env)); err == nil {
		t.Fatal("non-postgres scheme accepted")
	}

	env["DATABASE_URL"] = "postgresql://app@db:5432/app"
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

func writeSecret(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigReadsPasswordFile(t *testing.T) {
	env := baseEnv()
	delete(env, "DATABASE_PASSWORD")
	env["DATABASE_PASSWORD_FILE"] = writeSecret(t, "s3cret\n")
	cfg, err := Load(lookup(env))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DatabasePassword != "s3cret" {
		t.Errorf("DatabasePassword = %q, want the file contents without the trailing newline", cfg.DatabasePassword)
	}

	env["DATABASE_PASSWORD_FILE"] = filepath.Join(t.TempDir(), "missing")
	if _, err := Load(lookup(env)); err == nil || !strings.Contains(err.Error(), "DATABASE_PASSWORD_FILE") {
		t.Fatalf("err = %v, want unreadable DATABASE_PASSWORD_FILE", err)
	}
}

func TestConfigRejectsValueAndFile(t *testing.T) {
	env := baseEnv()
	env["DATABASE_PASSWORD_FILE"] = writeSecret(t, "other")
	_, err := Load(lookup(env))
	if err == nil || !strings.Contains(err.Error(), "only one of DATABASE_PASSWORD and DATABASE_PASSWORD_FILE") {
		t.Fatalf("err = %v, want both-set error", err)
	}
}

func TestConfigRequiresDatabasePassword(t *testing.T) {
	env := baseEnv()
	delete(env, "DATABASE_PASSWORD")
	if _, err := Load(lookup(env)); err == nil || !strings.Contains(err.Error(), "DATABASE_PASSWORD") {
		t.Fatalf("err = %v, want missing DATABASE_PASSWORD", err)
	}
}

func TestConfigRejectsPasswordInDatabaseURL(t *testing.T) {
	env := baseEnv()
	env["DATABASE_URL"] = "postgres://app:pw@db:5432/app"
	_, err := Load(lookup(env))
	if err == nil || !strings.Contains(err.Error(), "must not contain a password") {
		t.Fatalf("err = %v, want password-in-URL error", err)
	}
	if strings.Contains(err.Error(), "pw@") {
		t.Errorf("error leaks the URL: %v", err)
	}

	env["DATABASE_URL"] = "postgres://app@db:5432/app?password=pw"
	if _, err := Load(lookup(env)); err == nil || !strings.Contains(err.Error(), "must not contain a password") {
		t.Fatalf("err = %v, want password-query error", err)
	}
}

func TestLoadMigrate(t *testing.T) {
	env := map[string]string{
		"MIGRATE_DATABASE_URL":           "postgres://owner@db:5432/app?sslmode=disable",
		"DATABASE_PLAINTEXT":             "same-host",
		"MIGRATE_DATABASE_PASSWORD_FILE": writeSecret(t, "ownerpw\n"),
		"DB_APP_USER":                    "app",
	}
	cfg, err := LoadMigrate(lookup(env))
	if err != nil {
		t.Fatalf("LoadMigrate: %v", err)
	}
	if cfg.DatabasePassword != "ownerpw" || cfg.AppRole != "app" || cfg.Mode != "prod" {
		t.Errorf("cfg = %+v", cfg)
	}

	if _, err := LoadMigrate(lookup(map[string]string{})); err == nil {
		t.Fatal("LoadMigrate accepted an empty environment")
	} else {
		for _, key := range []string{"MIGRATE_DATABASE_URL", "MIGRATE_DATABASE_PASSWORD", "DB_APP_USER"} {
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error %q does not mention %s", err, key)
			}
		}
	}
}

func TestLoadSeed(t *testing.T) {
	env := baseEnv()
	env["ADMIN_ACCOUNT"] = "root"
	env["ADMIN_PASSWORD_FILE"] = writeSecret(t, "a long enough password\n")
	cfg, err := LoadSeed(lookup(env))
	if err != nil {
		t.Fatalf("LoadSeed: %v", err)
	}
	if cfg.AdminAccount != "root" || cfg.AdminPassword != "a long enough password" || cfg.DatabasePassword != "pw" {
		t.Errorf("cfg = %+v", cfg)
	}
	if _, err := LoadSeed(lookup(baseEnv())); err == nil {
		t.Fatal("LoadSeed accepted a missing admin account")
	} else {
		for _, key := range []string{"ADMIN_ACCOUNT", "ADMIN_PASSWORD"} {
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error %q does not mention %s", err, key)
			}
		}
	}
}

// A database connection verifies the server's certificate unless DATABASE_PLAINTEXT=same-host declares that the
// traffic stays on this host; a derived project pointed at a remote database cannot forget TLS.
func TestDatabaseURLRequiresVerifiedTLS(t *testing.T) {
	for _, tc := range []struct {
		url, plaintext string
		ok             bool
	}{
		{"postgres://app@db.example.com:5432/app?sslmode=verify-full", "", true},
		{"postgres://app@db:5432/app?sslmode=disable", "same-host", true},
		{"postgres://app@db:5432/app?sslmode=disable", "", false},
		{"postgres://app@db.example.com:5432/app?sslmode=require", "", false},
		{"postgres://app@db.example.com:5432/app", "", false},
		{"postgres://app@db:5432/app?sslmode=disable", "yes", false},
	} {
		for _, key := range []string{"DATABASE_URL", "MIGRATE_DATABASE_URL"} {
			err := loadWithURL(key, tc.url, tc.plaintext)
			if tc.ok && err != nil {
				t.Errorf("%s=%s DATABASE_PLAINTEXT=%q: %v", key, tc.url, tc.plaintext, err)
			}
			if !tc.ok && (err == nil || !strings.Contains(err.Error(), "sslmode=verify-full")) {
				t.Errorf("%s=%s DATABASE_PLAINTEXT=%q: err = %v, want the TLS requirement", key, tc.url, tc.plaintext, err)
			}
		}
	}
}

// loadWithURL loads serve's settings (DATABASE_URL) or migrate's (MIGRATE_DATABASE_URL) with url and, unless empty,
// DATABASE_PLAINTEXT.
func loadWithURL(key, url, plaintext string) error {
	env := map[string]string{key: url, "DATABASE_PASSWORD": "pw", "MIGRATE_DATABASE_PASSWORD": "pw", "DB_APP_USER": "app"}
	if plaintext != "" {
		env["DATABASE_PLAINTEXT"] = plaintext
	}
	if key == "DATABASE_URL" {
		_, err := Load(lookup(env))
		return err
	}
	_, err := LoadMigrate(lookup(env))
	return err
}
