package dbtest

import (
	"os"
	"regexp"
	"testing"

	"github.com/jackc/pgx/v5"
)

var pg *Server

func TestMain(m *testing.M) { Main(m, &pg) }

// NewDB repeats the database-level settings the initdb script applies, because CREATE DATABASE … TEMPLATE does
// not copy them. This keeps the copy equal to the script.
func TestNewDBMatchesInitDB(t *testing.T) {
	db := pg.NewDB(t)
	c, err := pgx.Connect(t.Context(), pg.SuperuserURL("postgres"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close(t.Context()) }()
	settings := func(name string) (owner, acl string) {
		t.Helper()
		err := c.QueryRow(t.Context(),
			`SELECT pg_get_userbyid(datdba), coalesce(datacl::text, '') FROM pg_database WHERE datname = $1`, name).
			Scan(&owner, &acl)
		if err != nil {
			t.Fatal(err)
		}
		return owner, acl
	}
	wantOwner, wantACL := settings(templateDB)
	gotOwner, gotACL := settings(db.Name)
	if wantOwner != OwnerRole {
		t.Errorf("initdb database owner = %q, want %q", wantOwner, OwnerRole)
	}
	if gotOwner != wantOwner || gotACL != wantACL {
		t.Errorf("test database owner=%q acl=%q, want owner=%q acl=%q", gotOwner, gotACL, wantOwner, wantACL)
	}
}

// Dependabot bumps the postgres image in docker-compose.yml only; tests must run the same image.
func TestImageMatchesCompose(t *testing.T) {
	compose, err := os.ReadFile("../../../../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^\s+image: (postgres:\S+)$`).FindSubmatch(compose)
	if m == nil {
		t.Fatal("docker-compose.yml has no postgres image")
	}
	if got := string(m[1]); got != Image {
		t.Errorf("docker-compose.yml postgres image = %q, dbtest.Image = %q", got, Image)
	}
}
