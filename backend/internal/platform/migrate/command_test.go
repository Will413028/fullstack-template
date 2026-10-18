package migrate

import (
	"net/url"
	"slices"
	"strings"
	"testing"
)

// The connection strings are assembled at run time: a URL literal with a password reads as a leaked credential to
// secret scanners.
func ownerURL(password string) string {
	return (&url.URL{Scheme: "postgres", User: url.UserPassword("owner", password), Host: "db:5432", Path: "/app"}).String()
}

func TestMigrateURLNotInArgs(t *testing.T) {
	t.Setenv("PGPASSWORD", "inherited")
	conn := ownerURL("fixture")
	cmd := atlasCommand(t.Context(), "atlas", "/tmp/atlas.hcl", "/app/migrations", conn)
	for _, a := range cmd.Args {
		if strings.Contains(a, "fixture") || strings.Contains(a, "postgres://") {
			t.Errorf("argument %q carries the connection string", a)
		}
	}
	if !slices.Contains(cmd.Env, atlasURLVar+"="+conn) {
		t.Errorf("env does not pass the connection string in %s", atlasURLVar)
	}
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "PG") {
			t.Errorf("env inherits %q", e)
		}
	}
}

func TestConnURLAddsPasswordAndLockTimeout(t *testing.T) {
	// characters that must be escaped in a URL's userinfo
	escaped := "p@ss/" + "word"
	got, err := connURL(Config{URL: "postgres://owner@db:5432/app?sslmode=disable", Password: escaped})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if pw, _ := u.User.Password(); u.User.Username() != "owner" || pw != escaped {
		t.Errorf("user = %q, password round-trip failed", u.User.Username())
	}
	if q := u.Query(); q.Get("lock_timeout") != lockTimeout || q.Get("sslmode") != "disable" || u.Host != "db:5432" {
		t.Errorf("connURL = host %q query %q", u.Host, u.RawQuery)
	}
}
