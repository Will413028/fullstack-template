// Package dbtest provides PostgreSQL for integration tests, set up the way the compose stack is: the same image,
// the same initdb script creating the owner and app roles (deploy/postgres/initdb/10-roles.sh), and migrations
// applied by internal/platform/migrate with the Atlas community build that ships in the image. Tests connect as
// the app role, so a query that only works with more privileges fails here as it would in production.
//
// Each test package starts one container; each test gets its own database. Docker being unavailable fails the
// tests instead of skipping them.
//
//	var pg *dbtest.Server
//	func TestMain(m *testing.M) { dbtest.Main(m, &pg) }
package dbtest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Will413028/fullstack-template/backend/internal/platform/migrate"
)

// Image is the PostgreSQL major version compose and the Atlas dev database use.
const Image = "postgres:18"

// Role names and passwords inside the test container.
const (
	templateDB = "fullstack"
	superuser  = "postgres"
	OwnerRole  = "fullstack_owner"
	AppRole    = "fullstack_app"
)

// Server is one test PostgreSQL container.
type Server struct {
	container *tcpostgres.PostgresContainer
	base      *url.URL
	seq       atomic.Int64
}

// DB is one test's database.
type DB struct {
	Name string
	// OwnerURL connects as the owner role, without a password (OwnerPassword); AppURL connects as the app role
	// and carries its password.
	OwnerURL      string
	OwnerPassword string
	AppURL        string
}

// Start starts a container initialized by the deploy initdb script.
func Start(ctx context.Context) (*Server, error) {
	c, err := tcpostgres.Run(ctx, Image,
		tcpostgres.WithDatabase(templateDB),
		tcpostgres.WithUsername(superuser),
		tcpostgres.WithPassword(superuser),
		tcpostgres.WithInitScripts(filepath.Join(repoRoot(), "deploy", "postgres", "initdb", "10-roles.sh")),
		testcontainers.WithEnv(map[string]string{"DB_OWNER_USER": OwnerRole, "DB_APP_USER": AppRole}),
		testcontainers.WithFiles(secret("db_owner_password", OwnerRole), secret("db_app_password", AppRole)),
		testcontainers.WithWaitStrategy(
			wait.ForAll(
				// the image restarts the server once after running the initdb scripts
				wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(90*time.Second),
				wait.ForListeningPort("5432/tcp"),
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("dbtest: start container: %w", err)
	}
	raw, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = c.Terminate(context.Background())
		return nil, fmt.Errorf("dbtest: connection string: %w", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		_ = c.Terminate(context.Background())
		return nil, fmt.Errorf("dbtest: parse connection string: %w", err)
	}
	return &Server{container: c, base: u}, nil
}

// secret mounts a password file where compose puts its secrets. The password equals the role name.
func secret(name, password string) testcontainers.ContainerFile {
	return testcontainers.ContainerFile{
		Reader:            strings.NewReader(password + "\n"),
		ContainerFilePath: "/run/secrets/" + name,
		FileMode:          0o444,
	}
}

// Close stops the container.
func (s *Server) Close() { _ = s.container.Terminate(context.Background()) }

// Main is a whole TestMain: start the container, run the tests, stop the container.
func Main(m *testing.M, s **Server) {
	srv, err := Start(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	*s = srv
	code := m.Run()
	srv.Close()
	os.Exit(code)
}

// URL connects to database name as user, with password unless it is empty.
func (s *Server) URL(name, user, password string) string {
	u := *s.base
	u.Path = "/" + name
	u.User = url.User(user)
	if password != "" {
		u.User = url.UserPassword(user, password)
	}
	return u.String()
}

// SuperuserURL connects to database name as the superuser, for test setup that needs it.
func (s *Server) SuperuserURL(name string) string { return s.URL(name, superuser, superuser) }

// NewDB creates an empty database for this test, configured like the one the initdb script set up, and drops it
// when the test ends.
func (s *Server) NewDB(t testing.TB) DB {
	t.Helper()
	name := fmt.Sprintf("t_%d", s.seq.Add(1))
	id := pgx.Identifier{name}.Sanitize()
	// The template copies what is inside the database (public owned by the owner role, no PUBLIC access);
	// database-level settings are not copied, so they are repeated here. TestNewDBMatchesInitDB keeps them equal.
	s.exec(t, "CREATE DATABASE "+id+" TEMPLATE "+templateDB+" OWNER "+OwnerRole,
		"REVOKE ALL ON DATABASE "+id+" FROM PUBLIC",
		"GRANT CONNECT ON DATABASE "+id+" TO "+AppRole)
	t.Cleanup(func() { s.exec(t, "DROP DATABASE "+id+" WITH (FORCE)") })
	return DB{
		Name:          name,
		OwnerURL:      s.URL(name, OwnerRole, ""),
		OwnerPassword: OwnerRole,
		AppURL:        s.URL(name, AppRole, AppRole),
	}
}

// Migrate applies dir to db as the owner role, the way `app migrate` does.
func (s *Server) Migrate(ctx context.Context, t testing.TB, db DB, dir string) error {
	t.Helper()
	return migrate.Run(ctx, migrate.Config{
		URL:           db.OwnerURL,
		Password:      db.OwnerPassword,
		AppRole:       AppRole,
		AtlasBin:      AtlasBin(),
		MigrationsDir: dir,
		Output:        t.Output(),
	})
}

// NewMigratedDB is NewDB with the repository's migrations applied.
func (s *Server) NewMigratedDB(t testing.TB) DB {
	t.Helper()
	db := s.NewDB(t)
	if err := s.Migrate(context.Background(), t, db, MigrationsDir()); err != nil {
		t.Fatal(err)
	}
	return db
}

func (s *Server) exec(t testing.TB, stmts ...string) {
	t.Helper()
	ctx := context.Background()
	c, err := pgx.Connect(ctx, s.SuperuserURL("postgres"))
	if err != nil {
		t.Fatalf("dbtest: connect as superuser: %v", err)
	}
	defer func() { _ = c.Close(ctx) }()
	for _, sql := range stmts {
		if _, err := c.Exec(ctx, sql); err != nil {
			t.Fatalf("dbtest: %s: %v", sql, err)
		}
	}
}

// AtlasBin is the Atlas community build `make tools` downloads, the same version and checksum as the image's;
// ATLAS_COMMUNITY_BIN overrides it.
func AtlasBin() string {
	if v := os.Getenv("ATLAS_COMMUNITY_BIN"); v != "" {
		return v
	}
	return filepath.Join(repoRoot(), "backend", "bin", "atlas-community")
}

// MigrationsDir is the repository's migrations directory.
func MigrationsDir() string { return filepath.Join(repoRoot(), "backend", "migrations") }

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
}
