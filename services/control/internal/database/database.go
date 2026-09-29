// Package database opens the sqlite store, applies the migrations, and hands
// back the bun handle the rest of the server queries. It uses the pure-Go
// modernc.org/sqlite driver so the binary stays cgo-free (CGO_ENABLED=0).
package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/migrate"
	_ "modernc.org/sqlite"

	"control/internal/database/migrations"
)

// Database is the server's handle on the store. It is a bun.DB, which is itself
// a *sql.DB, so the query builder and plain SQL reach the same connection — the
// escape hatch is always there, and taking it does not mean opening anything.
type Database struct {
	*bun.DB
}

// Open opens (creating if absent) the sqlite database at path, enables foreign
// keys and WAL, and applies every pending migration before returning.
func Open(path string) (*Database, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory %s: %w", dir, err)
		}
	}
	dataSourceName := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)",
		path,
	)
	handle, err := sql.Open("sqlite", dataSourceName)
	if err != nil {
		return nil, err
	}
	if err := handle.Ping(); err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}

	database := &Database{bun.NewDB(handle, sqlitedialect.New())}
	if err := database.migrate(context.Background()); err != nil {
		return nil, err
	}
	return database, nil
}

// migrate applies each pending migration exactly once. bun records what it has
// run in a table of its own, so this is safe to call on every start — which is
// the point: a server that boots is a server whose schema is current.
func (d *Database) migrate(ctx context.Context) error {
	migrator := migrate.NewMigrator(d.DB, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		return fmt.Errorf("prepare migrations: %w", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
