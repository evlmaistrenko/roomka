package passwordreset

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"control/internal/database"
	"control/internal/model"
)

func newStore(t *testing.T) *database.Database {
	t.Helper()
	// os.MkdirTemp with best-effort cleanup rather than t.TempDir: on Windows the
	// sqlite WAL/SHM files can still be locked when RemoveAll runs.
	dir, err := os.MkdirTemp("", "roomka-password-reset-test-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	store, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		store.Close()
		_ = os.RemoveAll(dir)
	})
	return store
}

func newUser(t *testing.T, store *database.Database, username, passwordHash string) int64 {
	t.Helper()
	now := time.Now().UTC()
	user := &model.User{
		Username:     username,
		DisplayName:  username,
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if _, err := store.NewInsert().Model(user).Exec(context.Background()); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return user.ID
}

func TestVerifyAcceptsAFreshSecret(t *testing.T) {
	store := newStore(t)
	userID := newUser(t, store, "quentin", "")

	secret, expiresAt, err := Issue(context.Background(), store, userID)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if remaining := time.Until(expiresAt); remaining <= 0 || remaining > TTL {
		t.Errorf("expiry is %s away, want at most the %s term", remaining, TTL)
	}

	info, err := Verify(context.Background(), store, secret)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if info.UserID != userID || info.Username != "quentin" {
		t.Errorf("info = %+v, want the user the secret was issued for", info)
	}
}

// TestTheSecretIsNotStored pins what the row keeps: a hash, never something
// a copy of the database could be used as.
func TestTheSecretIsNotStored(t *testing.T) {
	store := newStore(t)
	userID := newUser(t, store, "quentin", "")

	secret, _, err := Issue(context.Background(), store, userID)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	_, random, err := parse(secret)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	stored := read(t, store, userID)
	if stored.PasswordResetHash == "" || stored.PasswordResetHash == random || stored.PasswordResetHash == secret {
		t.Errorf("row keeps %q for secret %q, want a hash of it", stored.PasswordResetHash, secret)
	}
}

func TestVerifyRejectsWhatItShould(t *testing.T) {
	store := newStore(t)
	userID := newUser(t, store, "quentin", "original-hash")
	otherID := newUser(t, store, "rachel", "original-hash")

	valid, _, err := Issue(context.Background(), store, userID)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	_, random, err := parse(valid)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	issued := read(t, store, userID)

	cases := map[string]func(t *testing.T) string{
		"nonsense": func(*testing.T) string {
			return "not-a-secret"
		},
		"a guessed random part": func(*testing.T) string {
			guessed, err := randomPart()
			if err != nil {
				t.Fatalf("random: %v", err)
			}
			return format(userID, guessed)
		},
		"the right random part under another user's id": func(*testing.T) string {
			return format(otherID, random)
		},
		"a secret older than its term": func(t *testing.T) string {
			update(t, store, userID, "password_reset_issued_at = ?", time.Now().UTC().Add(-TTL-time.Minute))
			return valid
		},
		"a secret superseded by a later request": func(t *testing.T) string {
			if _, _, err := Issue(context.Background(), store, userID); err != nil {
				t.Fatalf("re-issue: %v", err)
			}
			return valid
		},
		"a secret issued before the password changed": func(t *testing.T) string {
			update(t, store, userID, "password_hash = ?", "new-hash")
			return valid
		},
		"a cleared secret": func(t *testing.T) string {
			if err := Clear(context.Background(), store, userID); err != nil {
				t.Fatalf("clear: %v", err)
			}
			return valid
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			// Each case starts from a known-good state, so that the case before
			// it cannot be the reason this one fails.
			restore(t, store, userID, issued)
			if _, err := Verify(context.Background(), store, valid); err != nil {
				t.Fatalf("the known-good secret stopped working: %v", err)
			}

			if _, err := Verify(context.Background(), store, build(t)); !errors.Is(err, ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func read(t *testing.T, store *database.Database, userID int64) model.User {
	t.Helper()
	var user model.User
	if err := store.NewSelect().
		Model(&user).
		Column("password_hash", "password_reset_hash", "password_reset_issued_at").
		Where("id = ?", userID).
		Scan(context.Background()); err != nil {
		t.Fatalf("read user: %v", err)
	}
	return user
}

func restore(t *testing.T, store *database.Database, userID int64, user model.User) {
	t.Helper()
	update(t, store, userID, "password_hash = ?", user.PasswordHash)
	update(t, store, userID, "password_reset_hash = ?", user.PasswordResetHash)
	update(t, store, userID, "password_reset_issued_at = ?", user.PasswordResetIssuedAt)
}

// update writes one column, through bun like the code under test, so that a
// value the test sets and a value the package sets are stored identically.
func update(t *testing.T, store *database.Database, userID int64, assignment string, value any) {
	t.Helper()
	if _, err := store.NewUpdate().
		Model((*model.User)(nil)).
		Set(assignment, value).
		Where("id = ?", userID).
		Exec(context.Background()); err != nil {
		t.Fatalf("set %s: %v", assignment, err)
	}
}
