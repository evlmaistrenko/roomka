package identity

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := VerifyPassword("correct horse battery staple", hash); err != nil {
		t.Errorf("verify correct password: %v", err)
	}
	if err := VerifyPassword("wrong", hash); err != ErrPasswordMismatch {
		t.Errorf("verify wrong password = %v, want ErrPasswordMismatch", err)
	}
}

// No usable hash is a mismatch like any other, reached after the same work, so
// neither the answer nor the time it takes says an account is missing.
func TestPasswordWithoutAHashIsAPlainMismatch(t *testing.T) {
	for _, encoded := range []string{"", "not a hash", "$argon2id$v=19$m=1,t=1,p=1$$"} {
		if err := VerifyPassword("anything", encoded); err != ErrPasswordMismatch {
			t.Errorf("verify against %q = %v, want ErrPasswordMismatch", encoded, err)
		}
	}
}

func TestPasswordHashesAreSalted(t *testing.T) {
	first, _ := HashPassword("same")
	second, _ := HashPassword("same")
	if first == second {
		t.Error("two hashes of the same password are identical — salt not applied")
	}
}

// TestGuessableCatchesTheObvious pins the rule the length requirement cannot
// state: twelve characters of nothing much is still nothing much.
func TestGuessableCatchesTheObvious(t *testing.T) {
	guessable := map[string]string{
		"a common word padded out": "mypasswordislong",
		"two characters repeated":  "abababababab",
		"one long run":             "abcdefghijkl",
		"the account it protects":  "quentin lives here",
	}
	for name, password := range guessable {
		t.Run(name, func(t *testing.T) {
			if !Guessable(password, "quentin", "Quentin Blake") {
				t.Errorf("%q was accepted", password)
			}
		})
	}

	acceptable := []string{
		"velvet harbour marmalade",
		"quiet lantern by the river",
		"Корневой Пользователь 42",
	}
	for _, password := range acceptable {
		if Guessable(password, "quentin", "Quentin Blake") {
			t.Errorf("%q was rejected", password)
		}
	}
}
