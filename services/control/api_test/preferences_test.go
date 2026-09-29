package apitest

import (
	"fmt"
	"strings"
	"testing"
)

const setPreferenceMutation = `mutation P($key: String!, $input: SetPreferenceInput!) {
	setPreference(key: $key, input: $input) { key value }
}`

// setPreference is the call the preference tests make over and over.
func (b *browser) setPreference(key string, value any) result {
	b.harness.t.Helper()
	return b.post(setPreferenceMutation, variables("key", key, "input", map[string]any{"value": value}))
}

// TestPreferencesRoundTripUntouched is the whole contract of the field: the
// server stores what it is given, hands it back unchanged, and never looks
// inside it.
func TestPreferencesRoundTripUntouched(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	const opaque = `{"theme":"dark","columns":[1,2,3]}`
	answer := root.setPreference("layout", opaque)
	expectOK(t, answer)
	if stored := preferences(answer.list("setPreference"))["layout"]; stored != opaque {
		t.Errorf("stored %q, want the value it was given back unchanged", stored)
	}

	// The whole set comes back every time, so a client holding one object never
	// has to merge.
	expectOK(t, root.setPreference("theme", "dark"))
	answer = root.post(meQuery, nil)
	expectOK(t, answer)
	if stored := preferences(answer.list("me.preferences")); len(stored) != 2 {
		t.Errorf("me carries %d preferences, want 2", len(stored))
	}

	answer = root.setPreference("theme", "light")
	expectOK(t, answer)
	if stored := preferences(answer.list("setPreference"))["theme"]; stored != "light" {
		t.Errorf("theme = %q after overwriting, want light", stored)
	}

	// A null value is the client saying "forget this", not "store nothing".
	answer = root.setPreference("theme", nil)
	expectOK(t, answer)
	stored := preferences(answer.list("setPreference"))
	if _, present := stored["theme"]; present {
		t.Error("a null value must remove the key")
	}
	if len(stored) != 1 {
		t.Errorf("%d preferences left, want 1", len(stored))
	}
}

func TestPreferencesBelongToOneAccount(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()
	member := harness.member(root, "quentin", 10, nil, nil)

	expectOK(t, root.setPreference("theme", "dark"))
	answer := member.post(meQuery, nil)
	expectOK(t, answer)
	if stored := preferences(answer.list("me.preferences")); len(stored) != 0 {
		t.Errorf("the member sees %v, want nothing of somebody else's", stored)
	}
}

// TestPreferencesAreCapped covers the one rule the server applies to a store the
// client defines: it may be filled, but not endlessly, and being full must not
// stop a client from working with what it already has.
func TestPreferencesAreCapped(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	for index := range 100 {
		answer := root.setPreference(fmt.Sprintf("key%d", index), "value")
		expectOK(t, answer)
	}

	expect(t, root.setPreference("one-too-many", "value"), "LIMIT_REACHED")

	// At the cap, overwriting and removing still work — only growing does not.
	expectOK(t, root.setPreference("key0", "a new value"))
	expectOK(t, root.setPreference("key0", nil))
	expectOK(t, root.setPreference("one-too-many", "value"))
}

// TestPreferenceConstraintsAreReportedPerArgument checks that a @constraint on
// an argument and one on an input field both name the thing that failed.
func TestPreferenceConstraintsAreReportedPerArgument(t *testing.T) {
	harness := newHarness(t)
	root := harness.signUp()

	cases := []struct {
		name      string
		key       string
		value     any
		reason    string
		inputPath []string
	}{
		{"an empty key", "", "value", "TOO_SHORT", []string{"key"}},
		{"a key past 64 characters", strings.Repeat("k", 65), "value", "TOO_LONG", []string{"key"}},
		{"an empty value", "theme", "", "TOO_SHORT", []string{"input", "value"}},
		{"a value past 4096 characters", "theme", strings.Repeat("v", 4097), "TOO_LONG", []string{"input", "value"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			answer := root.setPreference(testCase.key, testCase.value)
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

// preferences turns the schema's key/value list into a map, which is how a
// client thinks of it.
func preferences(listed []any) map[string]string {
	stored := map[string]string{}
	for _, one := range listed {
		preference, ok := one.(map[string]any)
		if !ok {
			continue
		}
		key, _ := preference["key"].(string)
		value, _ := preference["value"].(string)
		stored[key] = value
	}
	return stored
}
