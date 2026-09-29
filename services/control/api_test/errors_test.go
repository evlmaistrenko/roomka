package apitest

import (
	"net/http"
	"strings"
	"testing"

	"control/internal/config"
)

// The schema promises that every failure arrives in the same shape: a code from
// its own enum, and — where an argument is at fault — the argument's path and a
// reason. These tests are about that shape rather than about any one field.

// TestArgumentConstraintsAreEnforced walks @constraint across the three places
// it appears: on a plain argument, on an argument with a default, and on a field
// of an input object.
func TestArgumentConstraintsAreEnforced(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	cases := []struct {
		name      string
		document  string
		variables map[string]any
		reason    string
		inputPath []string
	}{
		{
			name:      "a limit below the floor",
			document:  `query S($limit: Int!) { sessions(limit: $limit) { id } }`,
			variables: variables("limit", 0),
			reason:    "OUT_OF_RANGE",
			inputPath: []string{"limit"},
		},
		{
			name:      "a limit above the ceiling",
			document:  `query U($limit: Int!) { users(limit: $limit) { totalCount } }`,
			variables: variables("limit", 101),
			reason:    "OUT_OF_RANGE",
			inputPath: []string{"limit"},
		},
		{
			name:      "a session term shorter than an hour",
			document:  `mutation L($input: LoginInput!) { login(input: $input) { user { id } } }`,
			variables: variables("input", map[string]any{"username": "root", "password": rootPassword, "sessionTtlSeconds": 60}),
			reason:    "OUT_OF_RANGE",
			inputPath: []string{"input", "sessionTtlSeconds"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			answer := root.post(testCase.document, testCase.variables)
			expect(t, answer, "INVALID_INPUT")
			if answer.reason() != testCase.reason {
				t.Errorf("reason = %q, want %q", answer.reason(), testCase.reason)
			}
			if got := strings.Join(answer.inputPath(), "."); got != strings.Join(testCase.inputPath, ".") {
				t.Errorf("inputPath = %v, want %v", answer.inputPath(), testCase.inputPath)
			}
		})
	}
}

// TestMalformedValuesAreReportedLikeConstraints is the example the schema's own
// ErrorCode documentation gives, transplanted onto a field that exists: a value
// that cannot be read at all is reported exactly like one that can be read and
// then rejected.
func TestMalformedValuesAreReportedLikeConstraints(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	answer := root.post(`query U($filter: UserFilterInput) { users(filter: $filter) { totalCount } }`,
		variables("filter", map[string]any{"createdAfter": "long ago"}))

	expect(t, answer, "INVALID_INPUT")
	if answer.reason() != "MALFORMED" {
		t.Errorf("reason = %q, want MALFORMED", answer.reason())
	}
	if got := strings.Join(answer.inputPath(), "."); got != "filter.createdAfter" {
		t.Errorf("inputPath = %v, want [filter createdAfter]", answer.inputPath())
	}
	if path := answer.Errors[0].Path; len(path) != 1 || path[0] != "users" {
		t.Errorf("path = %v, want [users] — the field that failed, not the value inside it", path)
	}
}

// TestInvalidDocumentsAreInvalidInput covers the failures that never reach a
// resolver. They are the client's fault too, and the schema has one code for
// that, so gqlgen's own vocabulary is translated into it.
func TestInvalidDocumentsAreInvalidInput(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	for name, document := range map[string]string{
		"a field that does not exist":       `query { nothingLikeThis }`,
		"a document that does not parse":    `query {`,
		"an enum value that does not exist": `query { users(sortBy: SIDEWAYS) { totalCount } }`,
	} {
		t.Run(name, func(t *testing.T) {
			expect(t, root.post(document, nil), "INVALID_INPUT")
		})
	}
}

// TestInternalFailuresSayNothing checks the one case where the server keeps
// something back. A failure the client cannot act on is reported as INTERNAL
// with nothing of the original in it — the original goes to the log instead.
func TestInternalFailuresSayNothing(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	// Something that cannot happen, made to happen: the table a field reads is
	// gone.
	harness.execute(`DROP TABLE user_preferences`)

	answer := root.setPreference("theme", "dark")
	expect(t, answer, "INTERNAL")
	if answer.message() != "internal server error" {
		t.Errorf("message = %q, want nothing of the original failure", answer.message())
	}
	if !strings.Contains(harness.console.text(), "user_preferences") {
		t.Error("the original failure must reach the log")
	}
}

// TestOriginIsCheckedWhenTheCookieTravels covers the rule the schema states in
// its very first comment. The session cookie is SameSite=Strict, so this is a
// second lock on the same door — and the one that still holds if a browser ever
// sends the cookie on a cross-site request anyway.
func TestOriginIsCheckedWhenTheCookieTravels(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	status, body := root.postRaw(`query { version }`, map[string]string{"Origin": "https://not-this-server.example"})
	if status != http.StatusForbidden {
		t.Errorf("status = %d (%s), want 403", status, strings.TrimSpace(body))
	}

	// A request with no Origin at all is not a browser request — nothing else
	// sets the header — so it is left to the cookie itself.
	status, _ = root.postRaw(`query { version }`, nil)
	if status != http.StatusOK {
		t.Errorf("status = %d without an Origin header, want 200", status)
	}

	// And a request that carries no cookie is nobody's session to steal.
	status, _ = harness.browser().postRaw(`query { version }`,
		map[string]string{"Origin": "https://not-this-server.example"})
	if status != http.StatusOK {
		t.Errorf("status = %d for an anonymous cross-origin call, want 200", status)
	}
}

// TestPagesThisServerServesAreAllowed covers the second origin the check lets
// through: the API's own. In development the playground lives there rather than
// on the UI's origin, and it would otherwise be locked out the moment somebody
// signed in — a page cannot forge the Origin it was addressed at, so allowing it
// gives nothing away.
func TestPagesThisServerServesAreAllowed(t *testing.T) {
	harness := newHarness(t, func(configuration *config.Config) {
		configuration.PublicURL = "https://ui.example"
	})

	// Every call the harness makes carries the server's own origin, so signing in
	// at all is the assertion: the UI lives somewhere else entirely.
	root := harness.signUp()
	expectOK(t, root.post(meQuery, nil))

	status, body := root.postRaw(`query { version }`,
		map[string]string{"Origin": "https://somewhere.else.example"})
	if status != http.StatusForbidden {
		t.Errorf("status = %d (%s) for a third origin, want 403", status, strings.TrimSpace(body))
	}
}
