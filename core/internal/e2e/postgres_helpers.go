//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	magicstore "github.com/kienbui1995/magic/core/internal/store"
)

// startPostgresContainer spins up an ephemeral Postgres 16 image that has
// the `vector` extension preinstalled (pgvector/pgvector:pg16). On success
// it registers a t.Cleanup that terminates the container and returns the
// connection URL.
//
// When Docker is not available (daemon not running, permission denied,
// not installed), the test is skipped — this lets local dev without Docker
// and restricted CI environments keep running the rest of the suite.
func startPostgresContainer(t *testing.T) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	ctr, err := tcpostgres.Run(ctx,
		"pgvector/pgvector:pg16",
		tcpostgres.WithDatabase("magic_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		t.Skipf("postgres container unavailable (docker required): %v", err)
	}

	t.Cleanup(func() {
		tctx, tcancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer tcancel()
		_ = ctr.Terminate(tctx)
	})

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}
	return connStr
}

// applyMigrations runs MagiC migrations in `direction` (up or down) against
// the given Postgres URL using the embedded migration FS.
func applyMigrations(t *testing.T, connStr, direction string) {
	t.Helper()
	src, err := iofs.New(magicstore.MigrationsFS(), "migrations")
	if err != nil {
		t.Fatalf("iofs.New: %v", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, connStr)
	if err != nil {
		t.Fatalf("migrate.NewWithSourceInstance: %v", err)
	}
	defer m.Close()

	switch direction {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			t.Fatalf("migrate.Up: %v", err)
		}
	case "down":
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			t.Fatalf("migrate.Down: %v", err)
		}
	default:
		t.Fatalf("unknown migration direction %q", direction)
	}
}

// setupPostgresStore brings up an ephemeral Postgres, applies migrations up,
// and returns a ready PostgreSQLStore plus its (non-superuser) connection
// string.
//
// RLS is not enforced for superusers, so migrations are applied as postgres
// but the returned store uses a freshly-created `magic_app` role (non-
// superuser, non-BYPASSRLS) — mirroring production posture.
func setupPostgresStore(t *testing.T) (*magicstore.PostgreSQLStore, string) {
	t.Helper()
	adminURL := startPostgresContainer(t)
	applyMigrations(t, adminURL, "up")

	appURL := createAppRole(t, adminURL, "magic_app", "apppw")

	s, err := magicstore.NewPostgreSQLStore(context.Background(), appURL)
	if err != nil {
		t.Fatalf("NewPostgreSQLStore: %v", err)
	}
	t.Cleanup(s.Close)
	return s, appURL
}

// createAppRole provisions a non-superuser role with the privileges MagiC
// needs (USAGE on schema, CRUD on every table) and returns a connection URL
// authenticated as that role. RLS is enforced for this role because it is
// neither a superuser nor a table owner.
func createAppRole(t *testing.T, adminURL, role, password string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	defer pool.Close()

	stmts := []string{
		fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD '%s'", role, password),
		fmt.Sprintf("GRANT USAGE ON SCHEMA public TO %s", role),
		fmt.Sprintf("GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO %s", role),
		fmt.Sprintf("GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO %s", role),
		fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %s", role),
	}
	for _, q := range stmts {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("create role %q: %v", q, err)
		}
	}

	// Rewrite the connection URL to use the new role.
	cfg, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		t.Fatalf("parse admin URL: %v", err)
	}
	u := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		role, password,
		cfg.ConnConfig.Host, cfg.ConnConfig.Port, cfg.ConnConfig.Database,
	)
	return u
}

// tableExists checks whether a table is visible in the current database.
func tableExists(ctx context.Context, s *magicstore.PostgreSQLStore, table string) (bool, error) {
	var exists bool
	err := s.Pool().QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
		table,
	).Scan(&exists)
	return exists, err
}

// queryCurrentSetting returns the session value of app.current_org_id on a
// freshly-acquired connection (must be acquired with an org-scoped context).
func queryCurrentSetting(ctx context.Context, s *magicstore.PostgreSQLStore) (string, error) {
	var got string
	if err := s.Pool().QueryRow(ctx,
		`SELECT COALESCE(current_setting('app.current_org_id', true), '')`,
	).Scan(&got); err != nil {
		return "", fmt.Errorf("query current_setting: %w", err)
	}
	return got, nil
}
