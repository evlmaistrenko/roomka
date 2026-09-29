// Package passwordreset issues and verifies the secret that lets somebody set a
// password without having one — a first sign-in, or a forgotten password.
//
// The secret is random, and the user row keeps only a hash of it, taken
// together with the password hash in force when it was issued, and the issue
// time. The secret itself is never stored, and nothing signs it, so there is no
// key that could forge one. Three properties fall out of the hash for free:
//
//   - a secret stops working the moment it is used, because using it changes
//     the password hash it was taken against;
//   - it stops working the same way when the password changes by any other
//     path, such as changePassword or an administrator's reset;
//   - issuing a new one invalidates the old, because it overwrites the hash.
package passwordreset

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"control/internal/database"
	"control/internal/model"
)

// TTL is how long a secret stays valid. It is short because the secret is the
// whole credential while it lives: anyone holding it can claim the account.
const TTL = 15 * time.Minute

// secretBytes is the randomness in a secret: as much as a session's, since the
// secret opens the account as surely as a session does.
const secretBytes = 32

// ErrInvalid covers every way a secret can fail — malformed, unknown user,
// superseded, already used, expired. The holder of a dead link can do nothing
// differently with a more specific answer, and a specific answer would confirm
// which user ids exist.
var ErrInvalid = errors.New("invalid password reset secret")

// Info is what a valid secret identifies: the account it opens and how long it
// stays open.
type Info struct {
	UserID    int64
	Username  string
	ExpiresAt time.Time
}

// Issue gives the user a fresh secret and returns it. Any secret issued
// earlier stops working.
func Issue(ctx context.Context, store *database.Database, userID int64) (string, time.Time, error) {
	random, err := randomPart()
	if err != nil {
		return "", time.Time{}, err
	}
	issuedAt := time.Now().UTC()

	err = store.RunInTx(ctx, nil, func(ctx context.Context, transaction bun.Tx) error {
		var passwordHash string
		if err := transaction.NewSelect().
			Model((*model.User)(nil)).
			Column("password_hash").
			Where("id = ?", userID).
			Scan(ctx, &passwordHash); err != nil {
			return err
		}
		_, err := transaction.NewUpdate().
			Model((*model.User)(nil)).
			Set("password_reset_hash = ?", hash(random, passwordHash)).
			Set("password_reset_issued_at = ?", issuedAt).
			Set("updated_at = ?", issuedAt).
			Where("id = ?", userID).
			Exec(ctx)
		return err
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return format(userID, random), issuedAt.Add(TTL), nil
}

// Verify checks a secret and reports which account it opens.
func Verify(ctx context.Context, store *database.Database, secret string) (Info, error) {
	userID, random, err := parse(secret)
	if err != nil {
		return Info{}, err
	}

	var user model.User
	err = store.NewSelect().
		Model(&user).
		Column("username", "password_hash", "password_reset_hash", "password_reset_issued_at").
		Where("id = ?", userID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Info{}, ErrInvalid
	}
	if err != nil {
		return Info{}, err
	}
	// No hash on the row: none was issued, or the last one was redeemed or
	// cleared. A hash that does not match: another secret replaced this one,
	// or the password changed since it was issued.
	if user.PasswordResetHash == "" ||
		subtle.ConstantTimeCompare([]byte(user.PasswordResetHash), []byte(hash(random, user.PasswordHash))) != 1 {
		return Info{}, ErrInvalid
	}
	expiresAt := user.PasswordResetIssuedAt.Add(TTL)
	if !time.Now().UTC().Before(expiresAt) {
		return Info{}, ErrInvalid
	}
	return Info{UserID: userID, Username: user.Username, ExpiresAt: expiresAt}, nil
}

// Clear retires the outstanding secret, if any, ahead of its expiry. Setting a
// password would retire it anyway; clearing also drops the issue time, which
// stops passwordResetInfo describing a secret that no longer works.
func Clear(ctx context.Context, store *database.Database, userID int64) error {
	_, err := store.NewUpdate().
		Model((*model.User)(nil)).
		Set("password_reset_hash = NULL").
		Set("password_reset_issued_at = NULL").
		Where("id = ?", userID).
		Exec(ctx)
	return err
}

// format renders the secret as "<user id>.<random>". The id tells Verify which
// row to compare against; on its own it opens nothing. Both parts survive a
// URL fragment, a console and a copy-paste unchanged.
func format(userID int64, random string) string {
	return strconv.FormatInt(userID, 10) + "." + random
}

func parse(secret string) (int64, string, error) {
	userIDText, random, ok := strings.Cut(secret, ".")
	if !ok || random == "" {
		return 0, "", ErrInvalid
	}
	userID, err := strconv.ParseInt(userIDText, 10, 64)
	if err != nil {
		return 0, "", ErrInvalid
	}
	return userID, random, nil
}

func randomPart() (string, error) {
	buffer := make([]byte, secretBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// hash is what the row keeps: the random part together with the password hash
// in force. A plain sha256 is enough, unlike for a password: the input carries
// 256 random bits, so there is nothing to guess even offline.
func hash(random, passwordHash string) string {
	digest := sha256.New()
	digest.Write([]byte(random))
	digest.Write([]byte{0})
	digest.Write([]byte(passwordHash))
	return base64.RawURLEncoding.EncodeToString(digest.Sum(nil))
}
