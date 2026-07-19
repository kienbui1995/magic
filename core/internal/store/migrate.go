package store

import (
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrationsFS exposes the embedded migration files so that tests can drive
// up/down directly via golang-migrate without duplicating the embed directive.
func MigrationsFS() embed.FS { return migrationsFS }

// RunMigrations applies all pending up migrations to the given PostgreSQL URL.
// It is idempotent — safe to call on every startup.
func RunMigrations(postgresURL string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("iofs.New: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, postgresURL)
	if err != nil {
		return fmt.Errorf("migrate.New: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate.Up: %w", err)
	}
	return nil
}
