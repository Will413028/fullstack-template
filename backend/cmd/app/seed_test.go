package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Will413028/fullstack-template/backend/internal/modules/auth"
	"github.com/Will413028/fullstack-template/backend/internal/platform/dbtest"
)

var pg *dbtest.Server

func TestMain(m *testing.M) { dbtest.Main(m, &pg) }

const adminPassword = "a long enough password"

func newSeedService(t *testing.T) (*auth.Service, *pgxpool.Pool) {
	t.Helper()
	db := pg.NewMigratedDB(t)
	pool, err := pgxpool.New(t.Context(), db.AppURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	svc, err := auth.New(t.Context(), pool, auth.Config{
		SessionAbsolute: time.Hour, SessionIdle: time.Hour,
		Hasher: auth.NewArgon2Hasher(auth.Argon2Params{MemoryKiB: 64, Time: 1, Threads: 1}, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, pool
}

func countUsers(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM users`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSeedAdminCreatesAdmin(t *testing.T) {
	svc, pool := newSeedService(t)
	var out bytes.Buffer
	if err := seedAdmin(t.Context(), svc, "Root", adminPassword, &out); err != nil {
		t.Fatal(err)
	}
	var role string
	if err := pool.QueryRow(t.Context(), `SELECT role FROM users WHERE account = 'root'`).Scan(&role); err != nil || role != "admin" {
		t.Fatalf("role = %q (%v), want admin", role, err)
	}
	if !strings.Contains(out.String(), `"root" created`) {
		t.Errorf("output %q", out.String())
	}
	if _, _, err := svc.SignIn(t.Context(), "root", adminPassword, ""); err != nil {
		t.Errorf("sign in as the seeded admin: %v", err)
	}
}

// The input goes through the contract's CreateUserRequest, like POST /api/users; there is no built-in default password.
func TestSeedAdminRejectsShortPassword(t *testing.T) {
	svc, pool := newSeedService(t)
	err := seedAdmin(t.Context(), svc, "root", "fourteen chars", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "ADMIN_PASSWORD: minimum string length is 15") {
		t.Fatalf("err = %v, want the password length rule", err)
	}
	if strings.Contains(err.Error(), "fourteen chars") {
		t.Errorf("error %q echoes the password", err)
	}
	if n := countUsers(t, pool); n != 0 {
		t.Errorf("%d users created", n)
	}
	err = seedAdmin(t.Context(), svc, "r!", adminPassword, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "ADMIN_ACCOUNT") {
		t.Fatalf("err = %v, want the account rules", err)
	}
	err = seedAdmin(t.Context(), svc, "root", "demon1q2w3e4r5t", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), auth.CodePasswordTooCommon) {
		t.Fatalf("err = %v, want the common-password rule", err)
	}
}

func TestSeedAdminExistingAccountUnchanged(t *testing.T) {
	svc, pool := newSeedService(t)
	if _, _, err := svc.CreateUser(t.Context(), "alice", "alice's own password"); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := pool.QueryRow(t.Context(), `SELECT role || password_hash FROM users`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := seedAdmin(t.Context(), svc, "ALICE", adminPassword, &out); err != nil {
		t.Fatalf("existing account: %v, want success", err)
	}
	var after string
	if err := pool.QueryRow(t.Context(), `SELECT role || password_hash FROM users`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before || !strings.HasPrefix(after, "user$argon2id") {
		t.Errorf("existing user changed: role and hash %q -> %q", before, after)
	}
	if !strings.Contains(out.String(), "already exists") {
		t.Errorf("output %q", out.String())
	}
}
