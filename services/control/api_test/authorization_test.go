package apitest

import "testing"

// operation is one call the tests make against the whole @auth matrix.
type operation struct {
	name      string
	document  string
	variables map[string]any
}

// guardedOperations is every field the schema marks with @auth, one call each.
// The list is written out rather than derived from the schema on purpose: a
// field that loses its directive should break a test, and a test that reads the
// directive to decide what to expect would pass either way.
func guardedOperations(userID, sessionID string) []operation {
	return []operation{
		{"sessions", `query { sessions { id } }`, nil},
		{"users", `query { users { totalCount } }`, nil},
		{"logout", `mutation { logout }`, nil},
		{"stepUp", `mutation S($input: StepUpInput!) { stepUp(input: $input) { user { id } } }`,
			variables("input", map[string]any{"password": memberPassword})},
		{"changePassword", `mutation C($input: ChangePasswordInput!) { changePassword(input: $input) { user { id } } }`,
			variables("input", map[string]any{"currentPassword": memberPassword, "newPassword": "a whole new phrase"})},
		{"updateProfile", `mutation U($input: UpdateProfileInput!) { updateProfile(input: $input) { user { id } } }`,
			variables("input", map[string]any{"displayName": "Someone Else"})},
		{"setPreference", `mutation P($input: SetPreferenceInput!) { setPreference(key: "theme", input: $input) { key } }`,
			variables("input", map[string]any{"value": "dark"})},
		{"revokeSession", `mutation R($sessionId: ID!) { revokeSession(sessionId: $sessionId) { id } }`,
			variables("sessionId", sessionID)},
		{"revokeOtherSessions", `mutation { revokeOtherSessions { id } }`, nil},
		{"createUser", `mutation C($input: CreateUserInput!) { createUser(input: $input) { id } }`,
			variables("input", map[string]any{"username": "newcomer", "displayName": "Newcomer", "rank": 1})},
		{"deleteUser", `mutation D($userId: ID!) { deleteUser(userId: $userId) }`, variables("userId", userID)},
		{"resetUserCredentials", `mutation R($userId: ID!) { resetUserCredentials(userId: $userId) { id } }`,
			variables("userId", userID)},
		{"setUserRoles", `mutation R($userId: ID!, $input: SetUserRolesInput!) { setUserRoles(userId: $userId, input: $input) { id } }`,
			variables("userId", userID, "input", map[string]any{"roles": []string{"ADMIN"}})},
		{"setUserPermissions", `mutation P($userId: ID!, $input: SetUserPermissionsInput!) { setUserPermissions(userId: $userId, input: $input) { id } }`,
			variables("userId", userID, "input", map[string]any{"permissions": []string{"USER_MANAGE"}})},
		{"setUserRank", `mutation R($userId: ID!, $input: SetUserRankInput!) { setUserRank(userId: $userId, input: $input) { id } }`,
			variables("userId", userID, "input", map[string]any{"rank": 5})},
	}
}

// TestGuardedFieldsNeedASession is the floor the whole schema stands on: with no
// credential, every field that carries @auth says so, and says the same thing.
func TestGuardedFieldsNeedASession(t *testing.T) {
	harness := newHarness(t)
	harness.signUp()
	anonymous := harness.browser()

	for _, call := range guardedOperations("1", "1") {
		t.Run(call.name, func(t *testing.T) {
			expect(t, anonymous.post(call.document, call.variables), "UNAUTHENTICATED")
		})
	}
}

// TestPublicFieldsNeedNothing is the other half of the same statement: the
// fields a client has to reach before it has a session are reachable.
func TestPublicFieldsNeedNothing(t *testing.T) {
	harness := newHarness(t)
	harness.signUp()
	anonymous := harness.browser()

	calls := []operation{
		{"version", `query { version }`, nil},
		{"setupRequired", `query { setupRequired }`, nil},
		{"me", meQuery, nil},
		{"passwordResetInfo", `query I($secret: String!) { passwordResetInfo(secret: $secret) { username } }`,
			variables("secret", "not-a-real-secret")},
		{"requestPasswordReset", `mutation { requestPasswordReset(username: "root") }`, nil},
	}
	for _, call := range calls {
		t.Run(call.name, func(t *testing.T) {
			expectOK(t, anonymous.post(call.document, call.variables))
		})
	}
}

// TestPermissionGuardsUserManagement checks the RBAC half of @auth: the fields
// that name USER_MANAGE are closed to a signed-in user who does not hold it, and
// the fields that name no permission stay open to them.
func TestPermissionGuardsUserManagement(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	member := harness.member(root, "quentin", 10, nil, nil)
	memberID := harness.userID("quentin")

	managementOnly := map[string]bool{
		"users": true, "createUser": true, "deleteUser": true,
		"resetUserCredentials": true, "setUserRoles": true,
		"setUserPermissions": true, "setUserRank": true,
	}
	for _, call := range guardedOperations(memberID, "1") {
		if !managementOnly[call.name] {
			continue
		}
		t.Run(call.name, func(t *testing.T) {
			expect(t, member.post(call.document, call.variables), "ACCESS_DENIED")
		})
	}

	// And the fields about yourself are not management: holding no permission at
	// all still leaves an account fully usable by its owner.
	expectOK(t, member.post(`query { sessions { id current } }`, nil))
	expectOK(t, member.post(`mutation P($input: SetPreferenceInput!) {
		setPreference(key: "theme", input: $input) { key value }
	}`, variables("input", map[string]any{"value": "dark"})))
}

// TestStepUpGuardsTheDestructiveHalf separates the two questions @auth asks.
// Everything that changes somebody else's account needs a password proved
// recently; reading the user list does not.
func TestStepUpGuardsTheDestructiveHalf(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	harness.member(root, "quentin", 10, nil, nil)
	memberID := harness.userID("quentin")
	harness.lapseStepUp()

	needsStepUp := map[string]bool{
		"createUser": true, "deleteUser": true, "resetUserCredentials": true,
		"setUserRoles": true, "setUserPermissions": true, "setUserRank": true,
	}
	for _, call := range guardedOperations(memberID, "1") {
		if !needsStepUp[call.name] {
			continue
		}
		t.Run(call.name, func(t *testing.T) {
			expect(t, root.post(call.document, call.variables), "STEP_UP_REQUIRED")
		})
	}

	t.Run("users stays readable", func(t *testing.T) {
		expectOK(t, root.post(`query { users { totalCount } }`, nil))
	})
}

// TestPermissionIsCheckedBeforeStepUp pins the order the two checks run in.
// Asking somebody for their password and then telling them it would not have
// helped is both rude and a hint about what exists behind the field.
func TestPermissionIsCheckedBeforeStepUp(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	member := harness.member(root, "quentin", 10, nil, nil)
	harness.lapseStepUp()

	answer := member.post(`mutation C($input: CreateUserInput!) { createUser(input: $input) { id } }`,
		variables("input", map[string]any{"username": "newcomer", "displayName": "Newcomer", "rank": 1}))
	expect(t, answer, "ACCESS_DENIED")
}

// TestRankDecidesWhoYouCanActOn is the rule that lives outside the schema. Every
// case here is a consequence of the same strict inequality, which is why they
// are one test.
func TestRankDecidesWhoYouCanActOn(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	manager := harness.member(root, "manager", 50, nil, []string{"USER_MANAGE"})
	harness.member(root, "peer", 50, nil, []string{"USER_MANAGE"})
	harness.member(root, "junior", 10, nil, nil)

	rootID := harness.userID("root")
	managerID := harness.userID("manager")
	peerID := harness.userID("peer")
	juniorID := harness.userID("junior")

	setRank := `mutation R($userId: ID!, $input: SetUserRankInput!) {
		setUserRank(userId: $userId, input: $input) { id rank }
	}`
	cases := []struct {
		name   string
		target string
		rank   int
		code   string
	}{
		{"someone below", juniorID, 20, ""},
		{"a peer at the same rank", peerID, 20, "ACCESS_DENIED"},
		{"someone above", rootID, 20, "ACCESS_DENIED"},
		{"themselves", managerID, 20, "ACCESS_DENIED"},
		{"someone below, to the caller's own rank", juniorID, 50, "ACCESS_DENIED"},
		{"someone below, to a rank above the caller", juniorID, 90, "ACCESS_DENIED"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			answer := manager.post(setRank, variables(
				"userId", testCase.target, "input", map[string]any{"rank": testCase.rank}))
			expect(t, answer, testCase.code)
		})
	}

	t.Run("creating a peer", func(t *testing.T) {
		answer := manager.post(createUserMutation, variables("input", map[string]any{
			"username": "newpeer", "displayName": "New Peer", "rank": 50,
		}))
		expect(t, answer, "ACCESS_DENIED")
	})
	t.Run("creating a subordinate", func(t *testing.T) {
		answer := manager.post(createUserMutation, variables("input", map[string]any{
			"username": "recruit", "displayName": "Recruit", "rank": 49,
		}))
		expectOK(t, answer)
	})
	t.Run("deleting someone above", func(t *testing.T) {
		expect(t, manager.post(`mutation D($userId: ID!) { deleteUser(userId: $userId) }`,
			variables("userId", rootID)), "ACCESS_DENIED")
	})
	t.Run("resetting the credentials of someone above", func(t *testing.T) {
		expect(t, manager.post(`mutation R($userId: ID!) { resetUserCredentials(userId: $userId) { id } }`,
			variables("userId", rootID)), "ACCESS_DENIED")
	})
	t.Run("the root user cannot even reach themselves", func(t *testing.T) {
		// Root is at a rank nobody can hold, including root, so the account that
		// can do everything cannot delete or demote itself either.
		expect(t, root.post(`mutation D($userId: ID!) { deleteUser(userId: $userId) }`,
			variables("userId", rootID)), "ACCESS_DENIED")
	})
}

// TestStaleUserIdsReadAsStale covers the ids that name nothing. They are the
// client's view being out of date, which is a different thing from being
// refused, and the schema has a reason for exactly that.
func TestStaleUserIdsReadAsStale(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	for name, userID := range map[string]string{
		"an id that never existed":   "9999",
		"an id that is not a number": "not-an-id",
	} {
		t.Run(name, func(t *testing.T) {
			answer := root.post(`mutation D($userId: ID!) { deleteUser(userId: $userId) }`,
				variables("userId", userID))
			expect(t, answer, "INVALID_INPUT")
			if answer.reason() != "TARGET_MISSING" {
				t.Errorf("reason = %q, want TARGET_MISSING", answer.reason())
			}
			if path := answer.inputPath(); len(path) != 1 || path[0] != "userId" {
				t.Errorf("inputPath = %v, want [userId]", path)
			}
		})
	}
}

// TestGrantsApplyToTheNextCall pins that permissions are read live. A user whose
// rights change mid-session must not have to sign in again for the change to
// mean anything — in either direction.
func TestGrantsApplyToTheNextCall(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	member := harness.member(root, "quentin", 10, nil, nil)
	memberID := harness.userID("quentin")

	expect(t, member.post(`query { users { totalCount } }`, nil), "ACCESS_DENIED")

	grant := `mutation P($userId: ID!, $input: SetUserPermissionsInput!) {
		setUserPermissions(userId: $userId, input: $input) { id permissions directPermissions }
	}`
	answer := root.post(grant, variables("userId", memberID,
		"input", map[string]any{"permissions": []string{"USER_MANAGE"}}))
	expectOK(t, answer)
	if granted := answer.list("setUserPermissions.directPermissions"); len(granted) != 1 {
		t.Fatalf("directPermissions = %v, want [USER_MANAGE]", granted)
	}
	expectOK(t, member.post(`query { users { totalCount } }`, nil))

	// And taking it away lands just as quickly.
	expectOK(t, root.post(grant, variables("userId", memberID,
		"input", map[string]any{"permissions": []string{}})))
	expect(t, member.post(`query { users { totalCount } }`, nil), "ACCESS_DENIED")
}

// TestRolesCarryTheirPermissions checks the half of RBAC that lives in the
// schema: ADMIN is not stored as a set of permissions anywhere, it is declared
// by @grants and read from the schema at startup.
func TestRolesCarryTheirPermissions(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	member := harness.member(root, "quentin", 10, nil, nil)
	memberID := harness.userID("quentin")

	answer := root.post(`mutation R($userId: ID!, $input: SetUserRolesInput!) {
		setUserRoles(userId: $userId, input: $input) { id roles directPermissions permissions }
	}`, variables("userId", memberID, "input", map[string]any{"roles": []string{"ADMIN"}}))
	expectOK(t, answer)

	if direct := answer.list("setUserRoles.directPermissions"); len(direct) != 0 {
		t.Errorf("directPermissions = %v, want none — the permission comes from the role", direct)
	}
	if effective := answer.list("setUserRoles.permissions"); len(effective) != 1 || effective[0] != "USER_MANAGE" {
		t.Errorf("permissions = %v, want [USER_MANAGE]", effective)
	}
	expectOK(t, member.post(`query { users { totalCount } }`, nil))
}
