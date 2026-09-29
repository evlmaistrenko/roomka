// Package identity holds the control server's security primitives: password hashing,
// the session cookie, and the request-context helpers that carry the
// authenticated caller. It has no database dependency — session state lives in
// package session.
package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters: OWASP's minimum recommendation (19 MiB, two passes, one
// lane). The cost is there for the day the database leaks, when guessing
// happens offline and no rate limit applies; online guessing is the rate
// limits' job. Hashes made with other parameters still verify, since each hash
// carries its own.
const (
	argonTime    = 2
	argonMemory  = 19 * 1024 // 19 MiB
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

// hashing admits one argon2 run per CPU at a time. Each run holds its memory for
// its whole duration, so without a cap a burst of sign-ins is a burst of
// allocations the size of the burst; with it, peak memory is fixed and the
// burst waits instead.
var hashing = make(chan struct{}, runtime.GOMAXPROCS(0))

func idKey(password, salt []byte, time, memory uint32, threads uint8, keyLength uint32) []byte {
	hashing <- struct{}{}
	defer func() { <-hashing }()
	return argon2.IDKey(password, salt, time, memory, threads, keyLength)
}

// ErrPasswordMismatch is returned by VerifyPassword when the password does not
// match — including when there was no usable hash to match it against.
var ErrPasswordMismatch = errors.New("password does not match")

// decoy is what a password is checked against when there is nothing real to
// check it against: an unknown user, or one whose password was never set.
// Answering those at once would tell a caller, by the time it took, which
// usernames exist and which accounts have a password.
var decoy = func() string {
	encoded, err := HashPassword("decoy")
	if err != nil {
		panic(err)
	}
	return encoded
}()

// HashPassword returns a PHC-formatted argon2id hash of password.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := idKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword checks password against a PHC-formatted argon2id hash. It
// returns nil on a match and ErrPasswordMismatch otherwise. An empty or
// malformed hash is a mismatch too, reached after the same work as a real
// check, so the time taken never says which case it was.
func VerifyPassword(password, encoded string) error {
	if err := verify(password, encoded); err != nil {
		if !errors.Is(err, ErrPasswordMismatch) {
			_ = verify(password, decoy)
		}
		return ErrPasswordMismatch
	}
	return nil
}

func verify(password, encoded string) error {
	fields := strings.Split(encoded, "$")
	if len(fields) != 6 || fields[1] != "argon2id" {
		return errors.New("malformed argon2id hash")
	}

	var version int
	if _, err := fmt.Sscanf(fields[2], "v=%d", &version); err != nil {
		return fmt.Errorf("argon2id version: %w", err)
	}
	if version != argon2.Version {
		return errors.New("incompatible argon2 version")
	}

	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(fields[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return fmt.Errorf("argon2id parameters: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(fields[4])
	if err != nil {
		return fmt.Errorf("argon2id salt: %w", err)
	}
	want, err := base64.RawStdEncoding.DecodeString(fields[5])
	if err != nil {
		return fmt.Errorf("argon2id hash: %w", err)
	}
	// Bounds before use: argon2 panics on an empty key or no lanes, and a hash
	// asking for gigabytes would take them. Every hash this server writes is
	// far inside these.
	if len(want) < 16 || len(salt) < 8 || threads == 0 || time == 0 || time > 10 ||
		memory < 8*uint32(threads) || memory > 256*1024 {
		return errors.New("argon2id parameters out of bounds")
	}

	got := idKey([]byte(password), salt, time, memory, threads, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrPasswordMismatch
	}
	return nil
}

// distinctRunesRequired is how much variety a password must have before length
// stops being a disguise: "abababababab" is twelve characters of two.
const distinctRunesRequired = 5

// guessable lists what a long password is usually made of when its author was
// only trying to clear a length rule. It is deliberately short: the length
// requirement does the real work, and a dictionary that pretends to be complete
// invites trust it cannot carry.
var guessable = []string{
	"password", "passw0rd", "letmein", "welcome", "qwerty", "asdfgh", "zxcvbn",
	"123456", "abc123", "iloveyou", "admin", "administrator", "changeme",
	"correcthorsebatterystaple",
}

// Guessable reports whether a password is too easy to guess despite passing the
// length rule. Personal values — the username and display name — are checked
// too: a password built out of the account it protects is guessable by anyone
// who can see the account, which on this server is anybody who can list users.
func Guessable(password string, personal ...string) bool {
	folded := strings.ToLower(password)
	if distinctRunes(folded) < distinctRunesRequired {
		return true
	}
	for _, common := range guessable {
		if strings.Contains(folded, common) {
			return true
		}
	}
	for _, value := range personal {
		value = strings.ToLower(strings.TrimSpace(value))
		// Two characters of an account name are a coincidence, not a leak.
		if len(value) >= 3 && strings.Contains(folded, value) {
			return true
		}
	}
	return isRun(folded)
}

// distinctRunes counts how many different characters a string is made of.
func distinctRunes(value string) int {
	seen := map[rune]bool{}
	for _, character := range value {
		seen[character] = true
	}
	return len(seen)
}

// isRun reports whether a password is one unbroken ascending or descending
// sequence of code points, which covers "123456789012" and "abcdefghijkl"
// without needing either in the list above.
func isRun(value string) bool {
	runes := []rune(value)
	if len(runes) < 2 {
		return false
	}
	ascending, descending := true, true
	for index := 1; index < len(runes); index++ {
		if !unicode.IsPrint(runes[index]) {
			return false
		}
		if runes[index] != runes[index-1]+1 {
			ascending = false
		}
		if runes[index] != runes[index-1]-1 {
			descending = false
		}
	}
	return ascending || descending
}
