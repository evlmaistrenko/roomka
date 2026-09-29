package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"control/internal/database/migrations"
)

// tempDBPath returns a database path in a fresh temp dir cleaned up best-effort
// (on Windows the sqlite WAL/SHM files can briefly lock, and t.TempDir would fail
// the test on that cleanup error).
func tempDBPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "roomka-db-test-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "test.db")
}

// TestOpenAppliesMigrations verifies a fresh database gets every table the
// server queries, and that it starts genuinely empty. There is deliberately no
// seed to assert: roles and the permissions they grant are declared in the
// GraphQL schema, not stored here.
func TestOpenAppliesMigrations(t *testing.T) {
	store, err := Open(tempDBPath(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	tableExists := `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`

	cases := []struct {
		name  string
		query string
		arg   any
		want  int
	}{
		{"users table", tableExists, "users", 1},
		{"user_roles table", tableExists, "user_roles", 1},
		{"user_permissions table", tableExists, "user_permissions", 1},
		{"sessions table", tableExists, "sessions", 1},
		{"user_preferences table", tableExists, "user_preferences", 1},
		{"no role catalog", tableExists, "roles", 0},
		{"no permission catalog", tableExists, "permissions", 0},
		{"users start empty", `SELECT COUNT(*) FROM users`, nil, 0},
	}
	for _, c := range cases {
		var got int
		var err error
		if c.arg == nil {
			err = store.QueryRow(c.query).Scan(&got)
		} else {
			err = store.QueryRow(c.query, c.arg).Scan(&got)
		}
		if err != nil {
			t.Fatalf("%s: query failed: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s = %d, want %d", c.name, got, c.want)
		}
	}
}

// TestCascadesFollowTheUser pins the foreign keys the base migration declares.
// Everything that belongs to a user has to go when they do — a session for an
// account that no longer exists is a credential nobody can revoke.
func TestCascadesFollowTheUser(t *testing.T) {
	store, err := Open(tempDBPath(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	if _, err := store.Exec(
		`INSERT INTO users (id, username, display_name, password_hash, rank, created_at, updated_at)
			VALUES (1, 'quentin', 'Quentin', '', 0, '2026-01-01 00:00:00+00:00', '2026-01-01 00:00:00+00:00')`,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	for _, statement := range []string{
		`INSERT INTO user_roles (user_id, role) VALUES (1, 'ADMIN')`,
		`INSERT INTO user_permissions (user_id, permission) VALUES (1, 'USER_MANAGE')`,
		`INSERT INTO user_preferences (user_id, name, value) VALUES (1, 'theme', 'dark')`,
		`INSERT INTO sessions (user_id, secret_hash, ttl_seconds, expires_at, created_at, last_used_at)
			VALUES (1, 'hash', 3600, '2026-01-01 01:00:00+00:00', '2026-01-01 00:00:00+00:00', '2026-01-01 00:00:00+00:00')`,
	} {
		if _, err := store.Exec(statement); err != nil {
			t.Fatalf("insert row: %v", err)
		}
	}

	if _, err := store.Exec(`DELETE FROM users WHERE id = 1`); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	for _, table := range []string{"user_roles", "user_permissions", "user_preferences", "sessions"} {
		var left int
		if err := store.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&left); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if left != 0 {
			t.Errorf("%d rows left in %s after their user was deleted", left, table)
		}
	}
}

// TestMigrateIsIdempotent verifies re-opening an existing database does not
// re-run migrations.
func TestMigrateIsIdempotent(t *testing.T) {
	path := tempDBPath(t)

	first, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	first.Close()

	second, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer second.Close()

	var applied int
	if err := second.NewSelect().
		Table("bun_migrations").
		ColumnExpr("COUNT(*)").
		Scan(context.Background(), &applied); err != nil {
		t.Fatalf("count applied migrations: %v", err)
	}
	// One row per migration, whatever their number: re-opening must record
	// nothing new, which is the property under test.
	if want := len(migrations.Migrations.Sorted()); applied != want {
		t.Errorf("applied migrations after reopen = %d, want %d (a migration must not re-run)", applied, want)
	}
}

// TestFoldCaseIsUnicodeAware covers the SQL function registered for search.
// sqlite's own lower() stops at ASCII, so without this a search for a Cyrillic
// name would be case-sensitive while a Latin one was not.
func TestFoldCaseIsUnicodeAware(t *testing.T) {
	store, err := Open(tempDBPath(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	cases := map[string]string{
		"ЖЕНЯ":     "женя",
		"Quentin":  "quentin",
		"ÅNGSTRÖM": "ångström",
		"already":  "already",
	}
	for input, want := range cases {
		var got string
		if err := store.QueryRow(`SELECT foldCase(?)`, input).Scan(&got); err != nil {
			t.Fatalf("foldCase(%q): %v", input, err)
		}
		if got != want {
			t.Errorf("foldCase(%q) = %q, want %q", input, got, want)
		}
	}

	var null any
	if err := store.QueryRow(`SELECT foldCase(NULL)`).Scan(&null); err != nil {
		t.Fatalf("foldCase(NULL): %v", err)
	}
	if null != nil {
		t.Errorf("foldCase(NULL) = %v, want NULL", null)
	}
}
