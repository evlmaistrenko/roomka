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

const key = "test-signing-key"

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

func newUser(t *testing.T, store *database.Database, passwordHash string) int64 {
	t.Helper()
	now := time.Now().UTC()
	user := &model.User{
		Username:     "quentin",
		DisplayName:  "Quentin",
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
	userID := newUser(t, store, "")

	secret, expiresAt, err := Issue(context.Background(), store, key, userID)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if remaining := time.Until(expiresAt); remaining <= 0 || remaining > TTL {
		t.Errorf("expiry is %s away, want at most the %s term", remaining, TTL)
	}

	info, err := Verify(context.Background(), store, key, secret)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if info.UserID != userID || info.Username != "quentin" {
		t.Errorf("info = %+v, want the user the secret was issued for", info)
	}
}

func TestVerifyRejectsWhatItShould(t *testing.T) {
	store := newStore(t)
	userID := newUser(t, store, "original-hash")

	valid, _, err := Issue(context.Background(), store, key, userID)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	issuedAt := storedIssue(t, store, userID)

	cases := map[string]func(t *testing.T) string{
		"nonsense": func(*testing.T) string {
			return "not-a-secret"
		},
		"a signature from another key": func(*testing.T) string {
			return format(userID, issuedAt, "original-hash", "some-other-key")
		},
		"a payload naming another user": func(*testing.T) string {
			return format(userID+1, issuedAt, "original-hash", key)
		},
		"a secret older than its term": func(t *testing.T) string {
			// Both halves are moved together — the stored issue time and the
			// time the secret is signed for — so nothing but the age is wrong.
			old := time.Now().UTC().Add(-TTL - time.Minute).Truncate(time.Second)
			setIssue(t, store, userID, old)
			return format(userID, old, "original-hash", key)
		},
		"a secret superseded by a later request": func(t *testing.T) string {
			earlier := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
			setIssue(t, store, userID, earlier)
			superseded := format(userID, earlier, "original-hash", key)
			if _, _, err := Issue(context.Background(), store, key, userID); err != nil {
				t.Fatalf("re-issue: %v", err)
			}
			return superseded
		},
		"a secret signed against a password since changed": func(t *testing.T) string {
			secret := format(userID, storedIssue(t, store, userID), "original-hash", key)
			update(t, store, userID, "password_hash = ?", "new-hash")
			return secret
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			// Each case starts from a known-good state, so that the case before
			// it cannot be the reason this one fails.
			reset(t, store, userID, issuedAt)
			if _, err := Verify(context.Background(), store, key, valid); err != nil {
				t.Fatalf("the known-good secret stopped working: %v", err)
			}

			if _, err := Verify(context.Background(), store, key, build(t)); !errors.Is(err, ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestClearRetiresAnOutstandingSecret(t *testing.T) {
	store := newStore(t)
	userID := newUser(t, store, "")

	secret, _, err := Issue(context.Background(), store, key, userID)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if err := Clear(context.Background(), store, userID); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := Verify(context.Background(), store, key, secret); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

func storedIssue(t *testing.T, store *database.Database, userID int64) time.Time {
	t.Helper()
	var issuedAt time.Time
	if err := store.NewSelect().
		Model((*model.User)(nil)).
		Column("password_reset_issued_at").
		Where("id = ?", userID).
		Scan(context.Background(), &issuedAt); err != nil {
		t.Fatalf("read issue time: %v", err)
	}
	return issuedAt
}

func setIssue(t *testing.T, store *database.Database, userID int64, at time.Time) {
	t.Helper()
	update(t, store, userID, "password_reset_issued_at = ?", at)
}

func reset(t *testing.T, store *database.Database, userID int64, issuedAt time.Time) {
	t.Helper()
	update(t, store, userID, "password_hash = ?", "original-hash")
	update(t, store, userID, "password_reset_issued_at = ?", issuedAt)
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
