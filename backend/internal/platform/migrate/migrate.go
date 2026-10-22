// Package migrate is the migrate subcommand: it applies migrations/ with the Atlas CLI as the owner role, then
// grants the app role DML on the public schema (docs/adr/2026-10-18-go-backend.md D5). Both steps can be re-run.
//
// A table whose comment starts with "append-only" (COMMENT ON TABLE in the module's schema.sql) gets INSERT and
// SELECT only, so the application cannot change or delete its rows; "append-only: retain N days by <column>" also
// deletes rows older than N days on every run, as the owner role.
//
// There is no in-house runner: Atlas honors `-- atlas:txmode none` (CREATE INDEX CONCURRENTLY) and records what it
// applied in atlas_schema_revisions. The connection string reaches Atlas only through its environment, never its
// command line, so the password does not show up in the process list.
package migrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// Config configures a run.
type Config struct {
	// URL connects as the owner role, without a password; Password is the owner role's password.
	URL      string
	Password string
	// AppRole is the role serve connects as; it gets DML only and must already exist (deploy/postgres/initdb).
	AppRole string
	// AtlasBin is the Atlas CLI; MigrationsDir holds the migrations and atlas.sum.
	AtlasBin      string
	MigrationsDir string
	// Output receives Atlas's output; nil means os.Stdout.
	Output io.Writer
}

// lockTimeout bounds how long a migration statement waits for a lock. A DDL statement queued behind a long
// transaction would otherwise block every read and write queued behind it; a timed-out migrate is simply re-run.
const lockTimeout = "5s"

// applyTimeout bounds one Atlas run.
const applyTimeout = 30 * time.Minute

// atlasURLVar is the environment variable the generated Atlas config reads the connection string from.
const atlasURLVar = "MIGRATE_ATLAS_URL"

// atlasConfig defines only the deploy env; generating migrations uses backend/atlas.hcl instead.
const atlasConfig = `env "deploy" {
  url = getenv("` + atlasURLVar + `")
}
`

// Run applies the migrations, then grants the app role.
func Run(ctx context.Context, cfg Config) error {
	if cfg.AppRole == "" {
		return errors.New("migrate: no app role")
	}
	u, err := connURL(cfg)
	if err != nil {
		return err
	}
	if err := apply(ctx, cfg, u); err != nil {
		return err
	}
	return grant(ctx, u, cfg.AppRole)
}

// connURL adds the password and lock_timeout to cfg.URL. Atlas's driver (lib/pq) and pgx both send unknown query
// parameters to the server as run-time parameters.
func connURL(cfg Config) (string, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		// the error would quote the URL
		return "", errors.New("migrate: malformed connection URL")
	}
	u.User = url.UserPassword(u.User.Username(), cfg.Password)
	q := u.Query()
	q.Set("lock_timeout", lockTimeout)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func apply(ctx context.Context, cfg Config, connURL string) error {
	dir, err := os.MkdirTemp("", "atlas-deploy-")
	if err != nil {
		return fmt.Errorf("migrate: temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	conf := filepath.Join(dir, "atlas.hcl")
	if err := os.WriteFile(conf, []byte(atlasConfig), 0o600); err != nil {
		return fmt.Errorf("migrate: write Atlas config: %w", err)
	}
	migrations, err := filepath.Abs(cfg.MigrationsDir)
	if err != nil {
		return fmt.Errorf("migrate: migrations dir: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, applyTimeout)
	defer cancel()
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}
	cmd := atlasCommand(ctx, cfg.AtlasBin, conf, migrations, connURL)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("migrate: atlas migrate apply: %w", err)
	}
	return nil
}

// atlasCommand builds the Atlas invocation. Its environment is only what Atlas needs: the inherited one could
// carry other secrets, and lib/pq rejects PG* variables it does not support.
func atlasCommand(ctx context.Context, bin, conf, migrations, connURL string) *exec.Cmd {
	//nolint:gosec // bin is deployment configuration, not user input
	cmd := exec.CommandContext(ctx, bin, "migrate", "apply",
		"--config", "file://"+conf, "--env", "deploy", "--dir", "file://"+migrations)
	cmd.Env = []string{atlasURLVar + "=" + connURL, "ATLAS_NO_UPDATE_NOTIFIER=1"}
	for _, k := range []string{"HOME", "TMPDIR"} {
		if v, ok := os.LookupEnv(k); ok {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	return cmd
}

// grant gives the app role DML on everything in public, now and later, except what appendOnly takes back. Default
// privileges cover only objects the owner role creates, which is every object migrations create. One transaction: a
// partial grant never commits.
func grant(ctx context.Context, connURL, role string) error {
	conn, err := pgx.Connect(ctx, connURL)
	if err != nil {
		return fmt.Errorf("migrate: connect: %w", err)
	}
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, role).Scan(&exists); err != nil {
		return fmt.Errorf("migrate: look up app role: %w", err)
	}
	if !exists {
		return fmt.Errorf("migrate: app role %q does not exist", role)
	}
	r := pgx.Identifier{role}.Sanitize()
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		for _, s := range []string{
			"GRANT USAGE ON SCHEMA public TO " + r,
			"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO " + r,
			"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO " + r,
			"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO " + r,
			"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO " + r,
		} {
			if _, err := tx.Exec(ctx, s); err != nil {
				return fmt.Errorf("migrate: grant app role: %w", err)
			}
		}
		return appendOnly(ctx, tx, r)
	})
}

// appendOnlyComment is the table comment appendOnly acts on; the retention part is optional.
var appendOnlyComment = regexp.MustCompile(`^append-only(?:: retain ([1-9][0-9]*) days by ([a-z_][a-z0-9_]*))?$`)

// appendOnly takes UPDATE, DELETE and TRUNCATE on every append-only table away from the app role r (already
// sanitized) and deletes the rows past each table's retention. A comment that starts with "append-only" but does not
// match the format fails the run, so a typo cannot leave a table writable or kept forever.
func appendOnly(ctx context.Context, tx pgx.Tx, r string) error {
	var tables []struct {
		Table   string `json:"table"`
		Comment string `json:"comment"`
	}
	if err := tx.QueryRow(ctx, `
		SELECT coalesce(json_agg(json_build_object('table', c.relname, 'comment', d.description) ORDER BY c.relname), '[]')
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_description d ON d.objoid = c.oid AND d.classoid = 'pg_class'::regclass AND d.objsubid = 0
		WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND d.description LIKE 'append-only%'`).
		Scan(&tables); err != nil {
		return fmt.Errorf("migrate: find append-only tables: %w", err)
	}
	for _, t := range tables {
		m := appendOnlyComment.FindStringSubmatch(t.Comment)
		if m == nil {
			return fmt.Errorf("migrate: table %s: comment %q is not \"append-only\" or \"append-only: retain N days by <column>\"", t.Table, t.Comment)
		}
		table := pgx.Identifier{t.Table}.Sanitize()
		if _, err := tx.Exec(ctx, "REVOKE UPDATE, DELETE, TRUNCATE ON "+table+" FROM "+r); err != nil {
			return fmt.Errorf("migrate: make %s append-only: %w", t.Table, err)
		}
		if m[1] == "" {
			continue
		}
		days, err := strconv.Atoi(m[1])
		if err != nil {
			return fmt.Errorf("migrate: table %s: retention %q: %w", t.Table, m[1], err)
		}
		if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE "+pgx.Identifier{m[2]}.Sanitize()+" < now() - make_interval(days => $1)", days); err != nil {
			return fmt.Errorf("migrate: prune %s: %w", t.Table, err)
		}
	}
	return nil
}
