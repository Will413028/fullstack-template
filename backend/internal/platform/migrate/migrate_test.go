package migrate_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Will413028/fullstack-template/backend/internal/platform/dbtest"
	"github.com/Will413028/fullstack-template/backend/internal/platform/migrate"
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

func migrated(t *testing.T, dir string) dbtest.DB {
	t.Helper()
	db := pg.NewDB(t)
	if err := pg.Migrate(t.Context(), t, db, dir); err != nil {
		t.Fatal(err)
	}
	return db
}

func wantInsufficientPrivilege(t *testing.T, err error, what string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" && pgErr.Code != "42P01" {
		t.Errorf("%s: err = %v, want permission denied", what, err)
	}
}

func TestAppRoleCannotDDL(t *testing.T) {
	db := migrated(t, "testdata/v1")
	app := connect(t, db.AppURL)
	ctx := t.Context()
	if _, err := app.Exec(ctx, `INSERT INTO widgets (name) VALUES ('a')`); err != nil {
		t.Fatalf("app INSERT: %v", err)
	}
	var n int
	if err := app.QueryRow(ctx, `SELECT count(*) FROM widgets`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("app SELECT: n=%d err=%v", n, err)
	}
	_, err := app.Exec(ctx, `CREATE TABLE intruder (id int)`)
	wantInsufficientPrivilege(t, err, "CREATE TABLE")
	_, err = app.Exec(ctx, `DROP TABLE widgets`)
	wantInsufficientPrivilege(t, err, "DROP TABLE")
	_, err = app.Exec(ctx, `CREATE SCHEMA intruder`)
	wantInsufficientPrivilege(t, err, "CREATE SCHEMA")
}

func TestMigrateGrantsIdempotent(t *testing.T) {
	db := migrated(t, "testdata/v1")
	if err := pg.Migrate(t.Context(), t, db, "testdata/v1"); err != nil {
		t.Fatalf("second run: %v", err)
	}
	// a table the owner creates after migrate ran (as the next migration would) is usable by the app role
	owner := connect(t, pg.URL(db.Name, dbtest.OwnerRole, db.OwnerPassword))
	if _, err := owner.Exec(t.Context(), `CREATE TABLE later (id bigint GENERATED ALWAYS AS IDENTITY, v text)`); err != nil {
		t.Fatal(err)
	}
	app := connect(t, db.AppURL)
	for _, q := range []string{
		`INSERT INTO widgets (name) VALUES ('a')`,
		`INSERT INTO later (v) VALUES ('a')`,
		`UPDATE later SET v = 'b'`,
		`DELETE FROM later`,
		`SELECT count(*) FROM widgets`,
	} {
		if _, err := app.Exec(t.Context(), q); err != nil {
			t.Errorf("app %s: %v", q, err)
		}
	}
}

func TestMigrateUnknownAppRole(t *testing.T) {
	db := pg.NewDB(t)
	err := migrate.Run(t.Context(), migrate.Config{
		URL: db.OwnerURL, Password: db.OwnerPassword, AppRole: "nobody",
		AtlasBin: dbtest.AtlasBin(), MigrationsDir: "testdata/v1", Output: t.Output(),
	})
	if err == nil || !strings.Contains(err.Error(), `"nobody" does not exist`) {
		t.Fatalf("err = %v, want unknown role", err)
	}
}

func TestMigrateTxmodeNone(t *testing.T) {
	db := migrated(t, "testdata/v2")
	var valid bool
	err := connect(t, db.AppURL).QueryRow(t.Context(),
		`SELECT indisvalid FROM pg_index WHERE indexrelid = 'widgets_name_idx'::regclass`).Scan(&valid)
	if err != nil || !valid {
		t.Fatalf("widgets_name_idx valid=%v err=%v", valid, err)
	}
}

func TestMigrateLockTimeout(t *testing.T) {
	db := migrated(t, "testdata/v1")
	holder := connect(t, pg.SuperuserURL(db.Name))
	tx, err := holder.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(t.Context(), `LOCK TABLE widgets IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	start := time.Now()
	err = pg.Migrate(ctx, t, db, "testdata/v2")
	if ctx.Err() != nil {
		t.Fatalf("migrate still waiting for the lock after %s: lock_timeout not applied", time.Since(start))
	}
	if err == nil {
		t.Fatal("migrate succeeded while the table was locked")
	}
}
