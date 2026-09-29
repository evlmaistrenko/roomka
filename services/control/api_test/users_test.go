package apitest

import (
	"fmt"
	"testing"
)

const usersQuery = `query Users($filter: UserFilterInput, $sortBy: UserSort, $after: String, $limit: Int!) {
	users(filter: $filter, sortBy: $sortBy, after: $after, limit: $limit) {
		items { id username displayName rank passwordSetAt roles }
		pageInfo { endCursor hasNextPage }
		totalCount
	}
}`

// account creates a user with no password, which is what createUser makes and
// what the listing tests need a lot of.
func (h *harness) account(root *browser, username, displayName string, rank int) {
	h.t.Helper()
	expectOK(h.t, root.post(createUserMutation, variables("input", map[string]any{
		"username": username, "displayName": displayName, "rank": rank,
	})))
}

// TestUsersPageCoversEverybodyExactlyOnce is the property keyset paging exists
// for. The users here are all created within the same second, so the sort value
// they are paged by is identical for all of them and only the id keeps the order
// total — which is exactly the case an offset would lose rows in.
func TestUsersPageCoversEverybodyExactlyOnce(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	for index := range 6 {
		harness.account(root, fmt.Sprintf("member%d", index), fmt.Sprintf("Member %d", index), index)
	}

	seen := map[string]int{}
	var after any
	pages := 0
	for {
		answer := root.post(usersQuery, variables("limit", 2, "after", after))
		expectOK(t, answer)
		pages++

		if total := answer.number("users.totalCount"); total != 7 {
			t.Fatalf("totalCount = %v, want 7 — every user, not every user on this page", total)
		}
		for _, item := range answer.list("users.items") {
			seen[item.(map[string]any)["username"].(string)]++
		}
		if answer.at("users.pageInfo.hasNextPage") != true {
			if answer.at("users.pageInfo.endCursor") == nil && len(answer.list("users.items")) > 0 {
				t.Error("a page with rows must carry a cursor")
			}
			break
		}
		after = answer.text("users.pageInfo.endCursor")
		if pages > 10 {
			t.Fatal("paging did not end")
		}
	}

	if pages != 4 {
		t.Errorf("walked %d pages of 2 over 7 users, want 4", pages)
	}
	if len(seen) != 7 {
		t.Errorf("saw %d distinct users, want 7", len(seen))
	}
	for username, count := range seen {
		if count != 1 {
			t.Errorf("%s appeared %d times, want once", username, count)
		}
	}
}

// TestUsersPagesInEveryOrder walks the page in each ordering the schema offers.
// The cursor carries the value it was sorted by, so every column it can be sorted
// by is a separate comparison against a separate kind of value — a rank is a
// number, a username is text — and a mistake in one of them would be invisible in
// the others.
func TestUsersPagesInEveryOrder(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	for index := range 5 {
		harness.account(root, fmt.Sprintf("member%d", index), fmt.Sprintf("Member %d", index), index*10)
	}

	for _, sortBy := range []string{"CREATED_DESC", "CREATED_ASC", "USERNAME_ASC", "USERNAME_DESC", "RANK_ASC", "RANK_DESC"} {
		t.Run(sortBy, func(t *testing.T) {
			var (
				walked []string
				after  any
			)
			for page := 0; page < 10; page++ {
				answer := root.post(usersQuery, variables("limit", 2, "after", after, "sortBy", sortBy))
				expectOK(t, answer)
				for _, item := range answer.list("users.items") {
					walked = append(walked, item.(map[string]any)["username"].(string))
				}
				if answer.at("users.pageInfo.hasNextPage") != true {
					break
				}
				after = answer.text("users.pageInfo.endCursor")
			}

			seen := map[string]bool{}
			for _, username := range walked {
				if seen[username] {
					t.Errorf("%s was listed twice", username)
				}
				seen[username] = true
			}
			if len(walked) != 6 {
				t.Errorf("walked %d users (%v), want all 6", len(walked), walked)
			}
		})
	}
}

// TestUsersCursorBelongsToOneQuery pins the schema's "only valid with the same
// filter and sortBy". A cursor is a position in one particular ordering of one
// particular set of rows; in any other it names nothing, and saying so is better
// than quietly paging through something else.
func TestUsersCursorBelongsToOneQuery(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	for index := range 4 {
		harness.account(root, fmt.Sprintf("member%d", index), fmt.Sprintf("Member %d", index), index)
	}

	answer := root.post(usersQuery, variables("limit", 2))
	expectOK(t, answer)
	cursor := answer.text("users.pageInfo.endCursor")

	answer = root.post(usersQuery, variables("limit", 2, "after", cursor, "sortBy", "USERNAME_ASC"))
	expect(t, answer, "INVALID_INPUT")
	if path := answer.inputPath(); len(path) != 1 || path[0] != "after" {
		t.Errorf("inputPath = %v, want [after]", path)
	}

	answer = root.post(usersQuery, variables("limit", 2, "after", cursor,
		"filter", map[string]any{"search": "member"}))
	expect(t, answer, "INVALID_INPUT")

	// And the same cursor with the same query still works.
	expectOK(t, root.post(usersQuery, variables("limit", 2, "after", cursor)))
}

// TestUsersSearchIgnoresCaseInAnyAlphabet is why the store carries a Unicode
// case fold of its own: sqlite's built-in one stops at ASCII, which would make
// the search work for "Member" and not for "Женя".
func TestUsersSearchIgnoresCaseInAnyAlphabet(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	harness.account(root, "evgeny", "Женя Майстренко", 10)
	harness.account(root, "quentin", "Quentin Blake", 10)

	cases := map[string]string{
		"an exact display name":      "Женя",
		"a lowercased display name":  "женя",
		"an uppercased display name": "ЖЕНЯ",
		"part of a username":         "EVGE",
	}
	for name, search := range cases {
		t.Run(name, func(t *testing.T) {
			answer := root.post(usersQuery, variables("limit", 10,
				"filter", map[string]any{"search": search}))
			expectOK(t, answer)
			if total := answer.number("users.totalCount"); total != 1 {
				t.Fatalf("searching %q found %v users, want 1", search, total)
			}
			if found := answer.list("users.items")[0].(map[string]any)["username"]; found != "evgeny" {
				t.Errorf("found %v, want evgeny", found)
			}
		})
	}

	// A wildcard is a character to look for, not a pattern to run.
	answer := root.post(usersQuery, variables("limit", 10, "filter", map[string]any{"search": "%"}))
	expectOK(t, answer)
	if total := answer.number("users.totalCount"); total != 0 {
		t.Errorf("searching for %%%% found %v users, want 0", total)
	}
}

func TestUsersFiltersNarrowTheCount(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	harness.account(root, "quentin", "Quentin", 10)
	harness.member(root, "signedin", 10, nil, nil)
	expectOK(t, root.post(`mutation R($userId: ID!, $input: SetUserRolesInput!) {
		setUserRoles(userId: $userId, input: $input) { id }
	}`, variables("userId", harness.userID("quentin"), "input", map[string]any{"roles": []string{"ADMIN"}})))

	cases := []struct {
		name   string
		filter map[string]any
		want   float64
	}{
		{"everybody", nil, 3},
		{"administrators", map[string]any{"role": "ADMIN"}, 2},
		{"accounts in use", map[string]any{"hasPassword": true}, 2},
		{"accounts never claimed", map[string]any{"hasPassword": false}, 1},
		{"a role and a state together", map[string]any{"role": "ADMIN", "hasPassword": false}, 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			answer := root.post(usersQuery, variables("limit", 10, "filter", testCase.filter))
			expectOK(t, answer)
			if total := answer.number("users.totalCount"); total != testCase.want {
				t.Errorf("totalCount = %v, want %v", total, testCase.want)
			}
		})
	}
}

func TestUsersSortInTheOrderAsked(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	harness.account(root, "bravo", "Bravo", 20)
	harness.account(root, "alpha", "Alpha", 30)

	cases := map[string][]string{
		"USERNAME_ASC":  {"alpha", "bravo", "root"},
		"USERNAME_DESC": {"root", "bravo", "alpha"},
		"RANK_DESC":     {"root", "alpha", "bravo"},
		"RANK_ASC":      {"bravo", "alpha", "root"},
	}
	for sortBy, want := range cases {
		t.Run(sortBy, func(t *testing.T) {
			answer := root.post(usersQuery, variables("limit", 10, "sortBy", sortBy))
			expectOK(t, answer)
			var got []string
			for _, item := range answer.list("users.items") {
				got = append(got, item.(map[string]any)["username"].(string))
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("order = %v, want %v", got, want)
			}
		})
	}
}

// TestCreatedAccountsWaitForTheirSecret covers the state a created user is in
// before anybody has claimed it: it exists, it is listed, and it cannot be
// signed into.
func TestCreatedAccountsWaitForTheirSecret(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	answer := root.post(createUserMutation, variables("input", map[string]any{
		"username": "quentin", "displayName": "Quentin", "rank": 10,
		"roles": []string{"ADMIN"}, "permissions": []string{"USER_MANAGE"},
	}))
	expectOK(t, answer)
	if answer.at("createUser.passwordSetAt") != nil {
		t.Error("a created account must have no password")
	}
	if roles := answer.list("createUser.roles"); len(roles) != 1 || roles[0] != "ADMIN" {
		t.Errorf("roles = %v, want the ones it was created with", roles)
	}

	// The account holds its rights from the moment it is created, but nobody can
	// use them until somebody redeems the secret.
	expect(t, harness.browser().post(loginMutation, variables("input", map[string]any{
		"username": "quentin", "password": memberPassword,
	})), "UNAUTHENTICATED")

	claimed := harness.browser()
	expectOK(t, claimed.post(setPasswordMutation, variables(
		"secret", harness.secretFor("quentin"),
		"input", map[string]any{"password": memberPassword})))
	expectOK(t, claimed.post(`query { users { totalCount } }`, nil))
}

func TestCreateUserRefusesATakenUsername(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	harness.account(root, "quentin", "Quentin", 10)

	answer := root.post(createUserMutation, variables("input", map[string]any{
		"username": "quentin", "displayName": "Another Quentin", "rank": 10,
	}))
	expect(t, answer, "INVALID_INPUT")
	if answer.reason() != "TAKEN" {
		t.Errorf("reason = %q, want TAKEN", answer.reason())
	}
	if path := answer.inputPath(); len(path) != 2 || path[1] != "username" {
		t.Errorf("inputPath = %v, want [input username]", path)
	}
}

// TestDeleteUserTakesEverythingWithThem checks the cascade the schema promises:
// the account's sessions and preferences go too, so nothing is left holding a
// credential for somebody who no longer exists.
func TestDeleteUserTakesEverythingWithThem(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	member := harness.member(root, "quentin", 10, nil, nil)
	expectOK(t, member.setPreference("theme", "dark"))

	expectOK(t, root.post(`mutation D($userId: ID!) { deleteUser(userId: $userId) }`,
		variables("userId", harness.userID("quentin"))))

	if answer := member.post(meQuery, nil); answer.at("me") != nil {
		t.Error("a deleted user's session must be gone")
	}
	answer := root.post(usersQuery, variables("limit", 10))
	expectOK(t, answer)
	if total := answer.number("users.totalCount"); total != 1 {
		t.Errorf("totalCount = %v, want 1", total)
	}

	var preferences int
	if err := harness.store.QueryRow(`SELECT COUNT(*) FROM user_preferences`).Scan(&preferences); err != nil {
		t.Fatalf("count preferences: %v", err)
	}
	if preferences != 0 {
		t.Errorf("%d preferences outlived their owner", preferences)
	}
}

// TestResetUserCredentialsTakesTheAccountBack is the answer to a compromised
// account: the password goes at once, every session with it, and a new secret is
// printed for handing to whoever should have it.
func TestResetUserCredentialsTakesTheAccountBack(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	member := harness.member(root, "quentin", 10, nil, nil)

	answer := root.post(`mutation R($userId: ID!) {
		resetUserCredentials(userId: $userId) { id username passwordSetAt }
	}`, variables("userId", harness.userID("quentin")))
	expectOK(t, answer)
	if answer.at("resetUserCredentials.passwordSetAt") != nil {
		t.Error("the account must be left without a password")
	}

	if answer := member.post(meQuery, nil); answer.at("me") != nil {
		t.Error("every session of a reset account must be over")
	}
	expect(t, harness.browser().post(loginMutation, variables("input", map[string]any{
		"username": "quentin", "password": memberPassword,
	})), "UNAUTHENTICATED")

	// The printed secret is the way back in, and it works against the account as
	// it is now — with no password at all.
	claimed := harness.browser()
	expectOK(t, claimed.post(setPasswordMutation, variables(
		"secret", harness.secretFor("quentin"),
		"input", map[string]any{"password": "gravel driveway sunset"})))
	if answer := claimed.post(meQuery, nil); answer.text("me.user.username") != "quentin" {
		t.Error("the reset secret must lead back into the same account")
	}
}
