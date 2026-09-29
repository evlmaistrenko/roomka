package session

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

const term = time.Hour

func newStore(t *testing.T) *database.Database {
	t.Helper()
	// os.MkdirTemp + best-effort cleanup rather than t.TempDir: on Windows the
	// sqlite WAL/SHM files can be briefly locked when RemoveAll runs, and
	// t.TempDir would fail the test on that cleanup error.
	dir, err := os.MkdirTemp("", "roomka-session-test-")
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

func newUser(t *testing.T, store *database.Database) int64 {
	t.Helper()
	now := time.Now().UTC()
	user := &model.User{
		Username:    "alice",
		DisplayName: "Alice",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := store.NewInsert().Model(user).Exec(context.Background()); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return user.ID
}

func issue(t *testing.T, store *database.Database, userID int64) Issued {
	t.Helper()
	issued, err := Issue(context.Background(), store, userID, term, Origin{UserAgent: "test"}, true)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return issued
}

func TestResolveAcceptsTheTokenItIssued(t *testing.T) {
	store := newStore(t)
	userID := newUser(t, store)
	issued := issue(t, store, userID)

	resolved, slid, err := Resolve(context.Background(), store, issued.Token, Origin{UserAgent: "browser"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.UserID != userID || resolved.ID != issued.Session.ID {
		t.Errorf("resolved session = (user %d, id %d), want (user %d, id %d)",
			resolved.UserID, resolved.ID, userID, issued.Session.ID)
	}
	if slid {
		t.Error("a session used at the start of its term must not slide")
	}
	if resolved.LastUserAgent != "browser" {
		t.Errorf("lastUserAgent = %q, want the agent of the request that used it", resolved.LastUserAgent)
	}
	if !resolved.StepUpExpiresAt.After(time.Now().UTC()) {
		t.Error("a session issued on a proved password must start raised")
	}
}

func TestResolveRejectsBadTokens(t *testing.T) {
	store := newStore(t)
	userID := newUser(t, store)
	issued := issue(t, store, userID)

	cases := map[string]string{
		"empty":           "",
		"unknown":         "wrong-secret",
		"extended":        issued.Token + "x",
		"the stored hash": issued.Session.SecretHash,
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Resolve(context.Background(), store, token, Origin{}); !errors.Is(err, ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestResolveRejectsEndedAndExpiredSessions(t *testing.T) {
	store := newStore(t)
	userID := newUser(t, store)

	ended := issue(t, store, userID)
	if err := End(context.Background(), store, ended.Session.ID); err != nil {
		t.Fatalf("end: %v", err)
	}
	if _, _, err := Resolve(context.Background(), store, ended.Token, Origin{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("ended session: err = %v, want ErrInvalid", err)
	}

	expired := issue(t, store, userID)
	expireAt(t, store, expired.Session.ID, time.Now().UTC().Add(-time.Minute))
	if _, _, err := Resolve(context.Background(), store, expired.Token, Origin{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("expired session: err = %v, want ErrInvalid", err)
	}
}

// TestResolveSlidesInTheLastThird pins the sliding rule: a session used with
// less than a third of its term left gets a whole new term, and the cookie has
// to be sent again to say so.
func TestResolveSlidesInTheLastThird(t *testing.T) {
	store := newStore(t)
	userID := newUser(t, store)
	issued := issue(t, store, userID)

	now := time.Now().UTC()
	expireAt(t, store, issued.Session.ID, now.Add(term/4))

	resolved, slid, err := Resolve(context.Background(), store, issued.Token, Origin{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !slid {
		t.Fatal("a session used in its last third must slide")
	}
	if remaining := time.Until(resolved.ExpiresAt); remaining < term-time.Minute {
		t.Errorf("remaining term after sliding = %s, want a full term of %s", remaining, term)
	}
}

func TestRaiseRecordsAProvedPassword(t *testing.T) {
	store := newStore(t)
	userID := newUser(t, store)
	issued, err := Issue(context.Background(), store, userID, term, Origin{}, false)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !issued.Session.StepUpExpiresAt.IsZero() {
		t.Fatal("a session issued without a proved password must not start raised")
	}

	if _, err := Raise(context.Background(), store, issued.Session.ID); err != nil {
		t.Fatalf("raise: %v", err)
	}
	raised, err := Get(context.Background(), store, issued.Session.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !raised.StepUpExpiresAt.After(time.Now().UTC()) {
		t.Errorf("stepUpExpiresAt = %s, want a moment in the future", raised.StepUpExpiresAt)
	}
}

func TestEndOthersKeepsTheOneAsking(t *testing.T) {
	store := newStore(t)
	userID := newUser(t, store)
	kept := issue(t, store, userID)
	first := issue(t, store, userID)
	second := issue(t, store, userID)

	ended, err := EndOthers(context.Background(), store, userID, kept.Session.ID)
	if err != nil {
		t.Fatalf("end others: %v", err)
	}
	if len(ended) != 2 {
		t.Errorf("ended %d sessions, want 2", len(ended))
	}
	if _, _, err := Resolve(context.Background(), store, kept.Token, Origin{}); err != nil {
		t.Errorf("the session that asked must survive: %v", err)
	}
	for name, token := range map[string]string{"first": first.Token, "second": second.Token} {
		if _, _, err := Resolve(context.Background(), store, token, Origin{}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s session: err = %v, want ErrInvalid", name, err)
		}
	}
}

// TestIssueEvictsTheLeastRecentlyUsed pins the cap's behaviour: signing in at
// the limit succeeds, and costs the session that has gone longest unused.
func TestIssueEvictsTheLeastRecentlyUsed(t *testing.T) {
	store := newStore(t)
	userID := newUser(t, store)

	tokens := make([]Issued, 0, MaxLivePerUser)
	for range MaxLivePerUser {
		tokens = append(tokens, issue(t, store, userID))
	}
	// The order is set explicitly rather than left to the order the rows were
	// written in, so the test is about the rule and not about how fast a loop
	// runs.
	for index, issued := range tokens {
		usedAt(t, store, issued.Session.ID, time.Now().UTC().Add(-time.Duration(len(tokens)-index)*time.Minute))
	}

	extra := issue(t, store, userID)
	if len(extra.Evicted) != 1 || extra.Evicted[0] != tokens[0].Session.ID {
		t.Errorf("evicted = %v, want [%d] (the least recently used)", extra.Evicted, tokens[0].Session.ID)
	}
	if _, _, err := Resolve(context.Background(), store, tokens[0].Token, Origin{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("the evicted session still resolves: %v", err)
	}
	if _, _, err := Resolve(context.Background(), store, extra.Token, Origin{}); err != nil {
		t.Errorf("the new session must work: %v", err)
	}

	live, err := List(context.Background(), store, userID, Filter{Live: pointer(true)}, SortCreatedDesc, 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(live) != MaxLivePerUser {
		t.Errorf("live sessions = %d, want the cap of %d", len(live), MaxLivePerUser)
	}
}

func TestListSeparatesLiveFromEnded(t *testing.T) {
	store := newStore(t)
	userID := newUser(t, store)
	live := issue(t, store, userID)
	ended := issue(t, store, userID)
	expired := issue(t, store, userID)

	if err := End(context.Background(), store, ended.Session.ID); err != nil {
		t.Fatalf("end: %v", err)
	}
	expireAt(t, store, expired.Session.ID, time.Now().UTC().Add(-time.Minute))

	cases := []struct {
		name   string
		filter Filter
		want   []int64
	}{
		{"everything", Filter{}, []int64{live.Session.ID, ended.Session.ID, expired.Session.ID}},
		{"live", Filter{Live: pointer(true)}, []int64{live.Session.ID}},
		// An expired session is not live either, which is the half of the rule
		// a naive "ended_at IS NOT NULL" would miss.
		{"not live", Filter{Live: pointer(false)}, []int64{ended.Session.ID, expired.Session.ID}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			listed, err := List(context.Background(), store, userID, testCase.filter, SortCreatedAsc, 100)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(listed) != len(testCase.want) {
				t.Fatalf("listed %d sessions, want %d", len(listed), len(testCase.want))
			}
			for index, one := range listed {
				if !contains(testCase.want, one.ID) {
					t.Errorf("session %d at index %d is not one of %v", one.ID, index, testCase.want)
				}
			}
		})
	}
}

func TestListFiltersByUserAgent(t *testing.T) {
	store := newStore(t)
	userID := newUser(t, store)
	if _, err := Issue(context.Background(), store, userID, term, Origin{UserAgent: "Firefox/1.0"}, true); err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := Issue(context.Background(), store, userID, term, Origin{UserAgent: "curl/8"}, true); err != nil {
		t.Fatalf("issue: %v", err)
	}

	listed, err := List(context.Background(), store, userID, Filter{UserAgent: pointer("firefox")}, SortCreatedDesc, 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 1 || listed[0].LastUserAgent != "Firefox/1.0" {
		t.Errorf("listed = %v, want the one Firefox session", listed)
	}

	// A wildcard is a character to search for, not a pattern to run.
	listed, err = List(context.Background(), store, userID, Filter{UserAgent: pointer("%")}, SortCreatedDesc, 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 0 {
		t.Errorf("searching for %%%% matched %d sessions, want 0", len(listed))
	}
}

// expireAt moves a session's expiry, which is how the tests reach states that
// would otherwise need the clock to move. It goes through bun like every other
// write: a time value written any other way would be in another format.
func expireAt(t *testing.T, store *database.Database, sessionID int64, at time.Time) {
	t.Helper()
	set(t, store, sessionID, "expires_at = ?", at)
}

func usedAt(t *testing.T, store *database.Database, sessionID int64, at time.Time) {
	t.Helper()
	set(t, store, sessionID, "last_used_at = ?", at)
}

func set(t *testing.T, store *database.Database, sessionID int64, assignment string, value any) {
	t.Helper()
	if _, err := store.NewUpdate().
		Model((*model.Session)(nil)).
		Set(assignment, value).
		Where("id = ?", sessionID).
		Exec(context.Background()); err != nil {
		t.Fatalf("set %s: %v", assignment, err)
	}
}

func pointer[T any](value T) *T { return &value }

func contains(values []int64, value int64) bool {
	for _, one := range values {
		if one == value {
			return true
		}
	}
	return false
}
