package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Will413028/fullstack-template/backend/internal/platform/dbtest"
)

var pg *dbtest.Server

func TestMain(m *testing.M) { dbtest.Main(m, &pg) }

func connect(t *testing.T, url string) *pgx.Conn {
	t.Helper()
	c, err := pgx.Connect(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

// The baseline is applied with the Atlas community build that ships in the image. Atlas keeps its revision table
// in its own schema, which the app role must not reach; the app role gets DML on the module's tables only.
func TestBaselineAppliesWithCommunity(t *testing.T) {
	db := pg.NewMigratedDB(t)
	c := connect(t, pg.SuperuserURL(db.Name))
	var revSchema string
	if err := c.QueryRow(t.Context(),
		`SELECT table_schema FROM information_schema.tables WHERE table_name = 'atlas_schema_revisions'`).
		Scan(&revSchema); err != nil {
		t.Fatalf("revision table: %v", err)
	}
	if revSchema != "atlas_schema_revisions" {
		t.Errorf("revision table in schema %q, want atlas_schema_revisions", revSchema)
	}
	for _, table := range []string{"users", "sessions", "login_throttles"} {
		var ok bool
		err := c.QueryRow(t.Context(), `SELECT has_table_privilege($1, 'public.'||$2, 'SELECT,INSERT,UPDATE,DELETE')`,
			dbtest.AppRole, table).Scan(&ok)
		if err != nil || !ok {
			t.Errorf("app role DML on %s = %v (%v), want true", table, ok, err)
		}
	}
	var usage bool
	if err := c.QueryRow(t.Context(), `SELECT has_schema_privilege($1, 'atlas_schema_revisions', 'USAGE')`,
		dbtest.AppRole).Scan(&usage); err != nil || usage {
		t.Errorf("app role USAGE on atlas_schema_revisions = %v (%v), want false", usage, err)
	}
}

func assertCheckViolation(t *testing.T, err error, what string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Errorf("%s: err = %v, want a check violation (23514)", what, err)
	}
}

// The DB checks back the contract (a second line of defense with the same values).
func TestSchemaChecks(t *testing.T) {
	db := pg.NewMigratedDB(t)
	c := connect(t, db.AppURL)
	ctx := t.Context()
	insertUser := `INSERT INTO users (account, password_hash, role) VALUES ($1, 'x', $2) RETURNING id`

	_, err := c.Exec(ctx, insertUser, "Alice", "user")
	assertCheckViolation(t, err, "uppercase account")
	_, err = c.Exec(ctx, insertUser, "al", "user")
	assertCheckViolation(t, err, "2-character account")
	_, err = c.Exec(ctx, insertUser, "alice", "guest")
	assertCheckViolation(t, err, "unknown role")
	_, err = c.Exec(ctx, `INSERT INTO login_throttles (account_key, failures, updated_at) VALUES ('a', -1, now())`)
	assertCheckViolation(t, err, "negative failures")
	_, err = c.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, created_at, last_used_at, expires_at)
		VALUES ('\x00', uuidv7(), now(), now(), now())`)
	assertCheckViolation(t, err, "short token hash")

	var id string
	if err := c.QueryRow(ctx, insertUser, "alice", "user").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, created_at, last_used_at, expires_at)
		VALUES (sha256('t'), $1, now(), now(), now() + interval '1 day')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Exec(ctx, `DELETE FROM users WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := c.QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&n); err != nil || n != 0 {
		t.Errorf("sessions after deleting their user = %d (%v), want 0", n, err)
	}
}

// undeletableFromUsers lists the tables whose foreign key to users neither cascades nor sets null: deleting an
// account would fail on their rows (DeleteUser) instead of removing what it owns.
func undeletableFromUsers(t *testing.T, c *pgx.Conn) []string {
	t.Helper()
	rows, err := c.Query(t.Context(), `
		SELECT conrelid::regclass::text FROM pg_constraint
		WHERE contype = 'f' AND confrelid = 'users'::regclass AND confdeltype NOT IN ('c', 'n')
		ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return tables
}

// Every table that references users declares what happens when the account is deleted: ON DELETE CASCADE for what
// the user owns, SET NULL for a reference that outlives it (docs/rules/database-design.md). A module that forgets
// would make account deletion fail for every user with a row there.
func TestForeignKeysToUsersAllowDeletion(t *testing.T) {
	db := pg.NewMigratedDB(t)
	c := connect(t, pg.SuperuserURL(db.Name))
	if bad := undeletableFromUsers(t, c); len(bad) > 0 {
		t.Errorf("foreign keys to users without ON DELETE CASCADE or SET NULL: %v", bad)
	}
	// the check itself: a table with a plain reference is reported
	if _, err := c.Exec(t.Context(), `CREATE TABLE probe (owner_id uuid REFERENCES users (id))`); err != nil {
		t.Fatal(err)
	}
	if bad := undeletableFromUsers(t, c); len(bad) != 1 || bad[0] != "probe" {
		t.Errorf("with a plain reference: %v, want [probe]", bad)
	}
}
