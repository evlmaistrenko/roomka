package apitest

import (
	"net/http"
	"strings"
	"testing"

	"control/internal/identity"
)

// TestSetupClaimsTheServerOnce covers the first thing that ever happens to this
// server: it is unclaimed, somebody claims it, and it can never be claimed
// again — which is the whole of the bootstrap problem.
func TestSetupClaimsTheServerOnce(t *testing.T) {
	harness := newHarness(t)
	anonymous := harness.browser()

	answer := anonymous.post(`query { setupRequired version }`, nil)
	expectOK(t, answer)
	if answer.at("setupRequired") != true {
		t.Fatal("a server with no users must report setupRequired")
	}
	if answer.text("version") != "test-version" {
		t.Errorf("version = %q, want the configured version", answer.text("version"))
	}

	root := harness.signUp()

	answer = anonymous.post(`query { setupRequired }`, nil)
	if answer.at("setupRequired") != false {
		t.Error("a server with a user must not report setupRequired")
	}

	answer = root.post(meQuery, nil)
	expectOK(t, answer)
	if got := answer.text("me.user.username"); got != "root" {
		t.Errorf("username = %q, want root", got)
	}
	if rank := answer.number("me.user.rank"); rank != 100 {
		t.Errorf("root rank = %v, want 100 — the rank no one else can reach", rank)
	}
	if roles := answer.list("me.user.roles"); len(roles) != 1 || roles[0] != "ADMIN" {
		t.Errorf("roles = %v, want [ADMIN]", roles)
	}
	if permissions := answer.list("me.permissions"); len(permissions) != 1 || permissions[0] != "USER_MANAGE" {
		t.Errorf("permissions = %v, want [USER_MANAGE] through the role", permissions)
	}

	// A second setup is not an error: it is the same question with a different
	// answer, and the answer is null.
	second := harness.browser().post(setupMutation, variables("input", map[string]any{
		"username":    "intruder",
		"displayName": "Intruder",
		"password":    "another long passphrase",
	}))
	expectOK(t, second)
	if second.at("setup") != nil {
		t.Error("setup must do nothing once a user exists")
	}
}

// TestSetupRejectsBadInput walks the three ways input can be wrong, and checks
// each is reported in the schema's own terms: a code, the argument at fault, and
// a reason.
func TestSetupRejectsBadInput(t *testing.T) {
	cases := []struct {
		name      string
		input     map[string]any
		reason    string
		inputPath []string
	}{
		{
			name:      "password below the length rule",
			input:     map[string]any{"username": "root", "displayName": "Root", "password": "short"},
			reason:    "TOO_SHORT",
			inputPath: []string{"input", "password"},
		},
		{
			name:      "username outside the pattern",
			input:     map[string]any{"username": "Root User", "displayName": "Root", "password": rootPassword},
			reason:    "MALFORMED",
			inputPath: []string{"input", "username"},
		},
		{
			name:      "password anybody would try",
			input:     map[string]any{"username": "root", "displayName": "Root", "password": "passwordpassword"},
			reason:    "TOO_GUESSABLE",
			inputPath: []string{"input", "password"},
		},
		{
			name:      "password built from the account",
			input:     map[string]any{"username": "root", "displayName": "Root", "password": "root of all things"},
			reason:    "TOO_GUESSABLE",
			inputPath: []string{"input", "password"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newHarness(t)
			answer := harness.browser().post(setupMutation, variables("input", testCase.input))

			expect(t, answer, "INVALID_INPUT")
			if answer.reason() != testCase.reason {
				t.Errorf("reason = %q, want %q", answer.reason(), testCase.reason)
			}
			if got := strings.Join(answer.inputPath(), "."); got != strings.Join(testCase.inputPath, ".") {
				t.Errorf("inputPath = %v, want %v", answer.inputPath(), testCase.inputPath)
			}
			if len(answer.Errors[0].Path) != 1 || answer.Errors[0].Path[0] != "setup" {
				t.Errorf("path = %v, want [setup] — the field that failed", answer.Errors[0].Path)
			}
		})
	}
}

// TestSessionCookieCarriesItsProtections pins the attributes the schema spells
// out. They are not decoration: the __Host- prefix is only honoured when all of
// them are right, so a browser would silently drop a cookie that got this wrong.
func TestSessionCookieCarriesItsProtections(t *testing.T) {
	harness := newHarness(t)
	answer := harness.browser().post(setupMutation, variables("input", map[string]any{
		"username":          "root",
		"displayName":       "Root User",
		"password":          rootPassword,
		"sessionTtlSeconds": 3600,
	}))
	expectOK(t, answer)

	cookie := answer.cookie(identity.SessionCookieName)
	if cookie == nil {
		t.Fatalf("no %s cookie was set", identity.SessionCookieName)
	}
	if !cookie.Secure || !cookie.HttpOnly {
		t.Errorf("cookie must be Secure and HttpOnly, got Secure=%v HttpOnly=%v", cookie.Secure, cookie.HttpOnly)
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Errorf("Path = %q, want / — required by the __Host- prefix", cookie.Path)
	}
	if cookie.MaxAge != 3600 {
		t.Errorf("Max-Age = %d, want the requested term of 3600", cookie.MaxAge)
	}
}

func TestLoginRejectsBadCredentialsWithoutSayingWhich(t *testing.T) {
	harness := newHarness(t)
	harness.signUp()

	wrongPassword := harness.browser().post(loginMutation, variables("input", map[string]any{
		"username": "root", "password": "not the right passphrase",
	}))
	unknownUser := harness.browser().post(loginMutation, variables("input", map[string]any{
		"username": "nobody", "password": rootPassword,
	}))

	expect(t, wrongPassword, "UNAUTHENTICATED")
	expect(t, unknownUser, "UNAUTHENTICATED")
	if wrongPassword.message() != unknownUser.message() {
		t.Errorf("a wrong password says %q and an unknown user says %q; the two must be indistinguishable",
			wrongPassword.message(), unknownUser.message())
	}
}

func TestLogoutEndsTheSessionAndTheCookie(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	stolen := root.sessionCookie()

	answer := root.post(`mutation { logout }`, nil)
	expectOK(t, answer)
	if cleared := answer.cookie(identity.SessionCookieName); cleared == nil || cleared.MaxAge >= 0 {
		t.Errorf("logout must clear the cookie, got %v", cleared)
	}

	answer = root.post(meQuery, nil)
	expectOK(t, answer)
	if answer.at("me") != nil {
		t.Error("me must be null once the session is over")
	}

	// The cookie the browser was holding is not merely forgotten: the session
	// behind it is gone, so presenting it again buys nothing.
	thief := harness.browser()
	thief.holds(stolen)
	answer = thief.post(meQuery, nil)
	expectOK(t, answer)
	if answer.at("me") != nil {
		t.Error("a logged-out session must not come back to life when its cookie is replayed")
	}
}

// TestStepUpRaisesTheSession covers the step-up cycle end to end: a lapsed
// session is turned away from a field that asks for one, proving the password
// raises it, and the same call then goes through unchanged.
func TestStepUpRaisesTheSession(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	harness.lapseStepUp()

	newUser := map[string]any{"username": "mallory", "displayName": "Mallory", "rank": 10}
	answer := root.post(createUserMutation, variables("input", newUser))
	expect(t, answer, "STEP_UP_REQUIRED")

	answer = root.post(`mutation StepUp($input: StepUpInput!) {
		stepUp(input: $input) { session { stepUpExpiresAt } }
	}`, variables("input", map[string]any{"password": "wrong passphrase entirely"}))
	expect(t, answer, "UNAUTHENTICATED")

	answer = root.post(`mutation StepUp($input: StepUpInput!) {
		stepUp(input: $input) { session { stepUpExpiresAt } }
	}`, variables("input", map[string]any{"password": rootPassword}))
	expectOK(t, answer)
	if answer.text("stepUp.session.stepUpExpiresAt") == "" {
		t.Error("a raised session must report when the raise lapses")
	}

	answer = root.post(createUserMutation, variables("input", newUser))
	expectOK(t, answer)
}

// TestChangePasswordEndsEveryOtherSession pins what a password change is for:
// the device you are on keeps working, and every other one stops.
func TestChangePasswordEndsEveryOtherSession(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	other := harness.browser()
	expectOK(t, other.post(loginMutation, variables("input", map[string]any{
		"username": "root", "password": rootPassword,
	})))

	const replacement = "marmalade on burnt toast"
	answer := root.post(`mutation ChangePassword($input: ChangePasswordInput!) {
		changePassword(input: $input) { session { current } }
	}`, variables("input", map[string]any{
		"currentPassword": rootPassword,
		"newPassword":     replacement,
	}))
	expectOK(t, answer)

	if answer := root.post(meQuery, nil); answer.at("me") == nil {
		t.Error("the session that changed the password must survive")
	}
	if answer := other.post(meQuery, nil); answer.at("me") != nil {
		t.Error("every other session must be over")
	}

	fresh := harness.browser()
	expectOK(t, fresh.post(loginMutation, variables("input", map[string]any{
		"username": "root", "password": replacement,
	})))
	expect(t, harness.browser().post(loginMutation, variables("input", map[string]any{
		"username": "root", "password": rootPassword,
	})), "UNAUTHENTICATED")
}

func TestChangePasswordNeedsTheCurrentOne(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	answer := root.post(`mutation ChangePassword($input: ChangePasswordInput!) {
		changePassword(input: $input) { user { id } }
	}`, variables("input", map[string]any{
		"currentPassword": "whatever it might be",
		"newPassword":     "marmalade on burnt toast",
	}))
	expect(t, answer, "UNAUTHENTICATED")
}

// TestPasswordResetIsSingleUse walks the whole reset path, including the two
// properties that come from never storing the secret: using it kills it, and
// asking again supersedes it.
func TestPasswordResetIsSingleUse(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	harness.member(root, "quentin", 10, nil, nil)

	answer := harness.browser().post(`mutation { requestPasswordReset(username: "quentin") }`, nil)
	expectOK(t, answer)
	first := harness.secretFor("quentin")

	answer = harness.browser().post(`query Info($secret: String!) {
		passwordResetInfo(secret: $secret) { username expiresAt }
	}`, variables("secret", first))
	expectOK(t, answer)
	if got := answer.text("passwordResetInfo.username"); got != "quentin" {
		t.Errorf("passwordResetInfo names %q, want quentin", got)
	}

	const replacement = "gravel driveway sunset"
	claimed := harness.browser()
	answer = claimed.post(setPasswordMutation, variables(
		"secret", first, "input", map[string]any{"password": replacement}))
	expectOK(t, answer)
	if answer.text("setPassword.user.passwordSetAt") == "" {
		t.Error("setPassword must record when the password was set")
	}

	// Redeeming it a second time cannot work: the hash it was signed against is
	// no longer the hash on the row. Supersession by a later request is the same
	// mechanism seen from the other side — see internal/password_reset, where a
	// test can put the two issues in different seconds.
	answer = harness.browser().post(setPasswordMutation, variables(
		"secret", first, "input", map[string]any{"password": "another good phrase"}))
	expect(t, answer, "INVALID_INPUT")
	if answer.reason() != "TARGET_MISSING" {
		t.Errorf("reason = %q, want TARGET_MISSING", answer.reason())
	}
	if answer := harness.browser().post(`query Info($secret: String!) {
		passwordResetInfo(secret: $secret) { username }
	}`, variables("secret", first)); answer.at("passwordResetInfo") != nil {
		t.Error("a spent secret must describe nothing")
	}
}

// TestRequestPasswordResetSaysNothingAboutWhoExists is the reason the mutation
// returns Void: the response is identical either way, and only the console
// differs.
func TestRequestPasswordResetSaysNothingAboutWhoExists(t *testing.T) {
	harness := newHarness(t)
	harness.signUp()

	known := harness.browser().post(`mutation { requestPasswordReset(username: "root") }`, nil)
	unknown := harness.browser().post(`mutation { requestPasswordReset(username: "ghost") }`, nil)

	expectOK(t, known)
	expectOK(t, unknown)
	if known.at("requestPasswordReset") != nil || unknown.at("requestPasswordReset") != nil {
		t.Error("requestPasswordReset returns Void, which is always null")
	}
	if strings.Contains(harness.console.text(), `"ghost"`) {
		t.Error("no secret may be printed for a username that does not exist")
	}
}

// TestSetPasswordEndsEverySessionTheUserHad covers the case the reset exists
// for: nobody is sure who else is holding the account.
func TestSetPasswordEndsEverySessionTheUserHad(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	member := harness.member(root, "quentin", 10, nil, nil)

	expectOK(t, harness.browser().post(`mutation { requestPasswordReset(username: "quentin") }`, nil))
	claimed := harness.browser()
	expectOK(t, claimed.post(setPasswordMutation, variables(
		"secret", harness.secretFor("quentin"),
		"input", map[string]any{"password": "gravel driveway sunset"})))

	if answer := member.post(meQuery, nil); answer.at("me") != nil {
		t.Error("the session the account had before the reset must be over")
	}
	if answer := claimed.post(meQuery, nil); answer.at("me") == nil {
		t.Error("setPassword must leave the caller signed in")
	}
}

func TestUpdateProfileChangesOnlyWhatItIsGiven(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	answer := root.post(`mutation UpdateProfile($input: UpdateProfileInput!) {
		updateProfile(input: $input) { user { username displayName } }
	}`, variables("input", map[string]any{"displayName": "Корневой Пользователь"}))
	expectOK(t, answer)
	if got := answer.text("updateProfile.user.displayName"); got != "Корневой Пользователь" {
		t.Errorf("displayName = %q, want the new one", got)
	}
	if got := answer.text("updateProfile.user.username"); got != "root" {
		t.Errorf("username = %q; usernames are immutable", got)
	}

	// The input says at least one value is required, and an empty one is the
	// client asking for nothing rather than asking to clear something.
	answer = root.post(`mutation UpdateProfile($input: UpdateProfileInput!) {
		updateProfile(input: $input) { user { id } }
	}`, variables("input", map[string]any{}))
	expect(t, answer, "INVALID_INPUT")
}
