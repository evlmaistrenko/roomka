package apitest

import (
	"testing"
	"time"

	"control/internal/identity"
)

// sessionID is the id of the session a browser is holding.
func (b *browser) sessionID() string {
	b.harness.t.Helper()
	answer := b.post(`query { me { session { id } } }`, nil)
	expectOK(b.harness.t, answer)
	id := answer.text("me.session.id")
	if id == "" {
		b.harness.t.Fatal("this browser is not signed in")
	}
	return id
}

// TestSessionsAreListedPerAccount covers what the sessions field is for: a
// person's own devices, and nobody else's — there is no permission in this
// schema that opens somebody else's list.
func TestSessionsAreListedPerAccount(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	member := harness.member(root, "quentin", 10, nil, nil)

	second := harness.browser()
	expectOK(t, second.post(loginMutation, variables("input", map[string]any{
		"username": "root", "password": rootPassword,
	})))

	answer := root.post(sessionsQuery, nil)
	expectOK(t, answer)
	if listed := answer.list("sessions"); len(listed) != 2 {
		t.Fatalf("root has %d sessions listed, want 2", len(listed))
	}
	if current := countCurrent(answer.list("sessions")); current != 1 {
		t.Errorf("%d sessions are marked current, want exactly 1", current)
	}

	answer = member.post(sessionsQuery, nil)
	expectOK(t, answer)
	if listed := answer.list("sessions"); len(listed) != 1 {
		t.Errorf("the member sees %d sessions, want only their own 1", len(listed))
	}
}

// TestRevokeSessionEndsThatDevice is the point of the whole sessions screen:
// seeing a device you do not recognise and being able to turn it off.
func TestRevokeSessionEndsThatDevice(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	other := harness.browser()
	expectOK(t, other.post(loginMutation, variables("input", map[string]any{
		"username": "root", "password": rootPassword,
	})))
	otherID := other.sessionID()

	answer := root.post(`mutation R($sessionId: ID!) {
		revokeSession(sessionId: $sessionId) { id current endedAt }
	}`, variables("sessionId", otherID))
	expectOK(t, answer)

	for _, listed := range answer.list("revokeSession") {
		session := listed.(map[string]any)
		if session["id"] == otherID && session["endedAt"] == nil {
			t.Error("the revoked session must be listed as ended")
		}
	}
	if answer := other.post(meQuery, nil); answer.at("me") != nil {
		t.Error("the revoked device must stop working")
	}
	if answer := root.post(meQuery, nil); answer.at("me") == nil {
		t.Error("the device that revoked must keep working")
	}
}

func TestRevokeSessionRefusesWhatIsNotYoursToEnd(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	member := harness.member(root, "quentin", 10, nil, nil)

	t.Run("the current session", func(t *testing.T) {
		answer := root.post(`mutation R($sessionId: ID!) { revokeSession(sessionId: $sessionId) { id } }`,
			variables("sessionId", root.sessionID()))
		expect(t, answer, "INVALID_INPUT")
		if answer.reason() != "MALFORMED" {
			t.Errorf("reason = %q, want MALFORMED — logout is the field for this", answer.reason())
		}
	})

	t.Run("somebody else's session", func(t *testing.T) {
		// Reported as missing rather than denied: the answer must not confirm
		// that a session id belongs to an account.
		answer := member.post(`mutation R($sessionId: ID!) { revokeSession(sessionId: $sessionId) { id } }`,
			variables("sessionId", root.sessionID()))
		expect(t, answer, "INVALID_INPUT")
		if answer.reason() != "TARGET_MISSING" {
			t.Errorf("reason = %q, want TARGET_MISSING", answer.reason())
		}
	})

	t.Run("a session that never existed", func(t *testing.T) {
		answer := root.post(`mutation R($sessionId: ID!) { revokeSession(sessionId: $sessionId) { id } }`,
			variables("sessionId", "9999"))
		expect(t, answer, "INVALID_INPUT")
		if answer.reason() != "TARGET_MISSING" {
			t.Errorf("reason = %q, want TARGET_MISSING", answer.reason())
		}
	})
}

func TestRevokeOtherSessionsKeepsTheOneAsking(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	for range 2 {
		other := harness.browser()
		expectOK(t, other.post(loginMutation, variables("input", map[string]any{
			"username": "root", "password": rootPassword,
		})))
	}

	answer := root.post(`mutation { revokeOtherSessions { id current endedAt } }`, nil)
	expectOK(t, answer)

	live := 0
	for _, listed := range answer.list("revokeOtherSessions") {
		session := listed.(map[string]any)
		if session["endedAt"] == nil {
			live++
			if session["current"] != true {
				t.Error("the only session left live must be the current one")
			}
		}
	}
	if live != 1 {
		t.Errorf("%d sessions are still live, want 1", live)
	}
}

// TestSessionsFilterSeparatesLiveFromEnded checks the filter that a sessions
// screen actually uses, including the half that is easy to get wrong: an expired
// session is not live either.
func TestSessionsFilterSeparatesLiveFromEnded(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	ended := harness.browser()
	expectOK(t, ended.post(loginMutation, variables("input", map[string]any{
		"username": "root", "password": rootPassword,
	})))
	endedID := ended.sessionID()
	expectOK(t, root.post(`mutation R($sessionId: ID!) { revokeSession(sessionId: $sessionId) { id } }`,
		variables("sessionId", endedID)))

	expired := harness.browser()
	expectOK(t, expired.post(loginMutation, variables("input", map[string]any{
		"username": "root", "password": rootPassword,
	})))
	harness.expireSession(expired.sessionID(), time.Now().UTC().Add(-time.Hour))

	live := root.post(sessionsQuery, variables("filter", map[string]any{"live": true}))
	expectOK(t, live)
	if listed := live.list("sessions"); len(listed) != 1 {
		t.Errorf("live sessions = %d, want 1", len(listed))
	}

	notLive := root.post(sessionsQuery, variables("filter", map[string]any{"live": false}))
	expectOK(t, notLive)
	if listed := notLive.list("sessions"); len(listed) != 2 {
		t.Errorf("sessions that are not live = %d, want 2 (one ended, one expired)", len(listed))
	}
}

// TestSessionSlidesInItsLastThird pins the sliding rule as a client sees it: the
// expiry moves and the cookie is sent again, so the browser's copy and the
// server's agree.
func TestSessionSlidesInItsLastThird(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	before := root.post(meQuery, nil)
	expectOK(t, before)
	if cookie := before.cookie(identity.SessionCookieName); cookie != nil {
		t.Error("a session used early in its term must not be re-sent to the browser")
	}

	// A day-long session with six hours left is in its last third.
	harness.expireSessions(time.Now().UTC().Add(6 * time.Hour))

	after := root.post(meQuery, nil)
	expectOK(t, after)
	cookie := after.cookie(identity.SessionCookieName)
	if cookie == nil {
		t.Fatal("a session that slid must be sent to the browser again")
	}
	if cookie.MaxAge != 86400 {
		t.Errorf("Max-Age = %d, want a restarted term of 86400", cookie.MaxAge)
	}

	expiresAt, err := time.Parse(time.RFC3339, after.text("me.session.expiresAt"))
	if err != nil {
		t.Fatalf("parse expiresAt %q: %v", after.text("me.session.expiresAt"), err)
	}
	if remaining := time.Until(expiresAt); remaining < 23*time.Hour {
		t.Errorf("remaining term = %s, want a full day", remaining)
	}
}

// TestExpiredSessionsAreTurnedAway covers what happens after the sliding stops
// happening: the cookie is still in the browser, and it buys nothing.
func TestExpiredSessionsAreTurnedAway(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	harness.expireSessions(time.Now().UTC().Add(-time.Second))

	answer := root.post(meQuery, nil)
	expectOK(t, answer)
	if answer.at("me") != nil {
		t.Error("an expired session must not identify anybody")
	}
	if cleared := answer.cookie(identity.SessionCookieName); cleared == nil || cleared.MaxAge >= 0 {
		t.Error("a cookie that will never work again must be cleared from the browser")
	}
	expect(t, root.post(`query { sessions { id } }`, nil), "UNAUTHENTICATED")
}

func countCurrent(sessions []any) int {
	count := 0
	for _, listed := range sessions {
		if session, ok := listed.(map[string]any); ok && session["current"] == true {
			count++
		}
	}
	return count
}
