// Package passwordreset issues and verifies the secret that lets somebody set a
// password without having one — a first sign-in, or a forgotten password.
//
// The secret is never stored. It is a signature over the user's id, the moment
// it was issued, and the password hash in force at that moment; the user row
// keeps only the issue time. Two properties fall out of that for free: a secret
// stops working the moment it is used, because using it changes the hash it was
// signed against, and issuing a new one invalidates the old, because the stored
// issue time no longer matches.
package passwordreset

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
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

// Issue records a fresh issue time on the user and returns the secret signed
// against it. Any secret issued earlier stops working.
func Issue(ctx context.Context, store *database.Database, key string, userID int64) (string, time.Time, error) {
	// Truncated to the second because that is the resolution the secret carries
	// in its payload; signing a more precise time than it can hold would make
	// every secret fail verification.
	issuedAt := time.Now().UTC().Truncate(time.Second)

	var passwordHash string
	err := store.RunInTx(ctx, nil, func(ctx context.Context, transaction bun.Tx) error {
		if err := transaction.NewSelect().
			Model((*model.User)(nil)).
			Column("password_hash").
			Where("id = ?", userID).
			Scan(ctx, &passwordHash); err != nil {
			return err
		}
		_, err := transaction.NewUpdate().
			Model((*model.User)(nil)).
			Set("password_reset_issued_at = ?", issuedAt).
			Set("updated_at = ?", time.Now().UTC()).
			Where("id = ?", userID).
			Exec(ctx)
		return err
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return format(userID, issuedAt, passwordHash, key), issuedAt.Add(TTL), nil
}

// Verify checks a secret and reports which account it opens.
func Verify(ctx context.Context, store *database.Database, key, secret string) (Info, error) {
	userID, issuedAt, signature, err := parse(secret)
	if err != nil {
		return Info{}, err
	}

	var user model.User
	err = store.NewSelect().
		Model(&user).
		Column("username", "password_hash", "password_reset_issued_at").
		Where("id = ?", userID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Info{}, ErrInvalid
	}
	if err != nil {
		return Info{}, err
	}
	// A secret whose issue time is not the one on the row was superseded by a
	// later request, or was already used — setPassword clears the column.
	if user.PasswordResetIssuedAt.IsZero() || !user.PasswordResetIssuedAt.Equal(issuedAt) {
		return Info{}, ErrInvalid
	}
	if !hmac.Equal([]byte(signature), []byte(sign(userID, issuedAt, user.PasswordHash, key))) {
		return Info{}, ErrInvalid
	}
	expiresAt := issuedAt.Add(TTL)
	if !time.Now().UTC().Before(expiresAt) {
		return Info{}, ErrInvalid
	}
	return Info{UserID: userID, Username: user.Username, ExpiresAt: expiresAt}, nil
}

// Clear drops the issue time, which retires the secret ahead of its expiry.
func Clear(ctx context.Context, store *database.Database, userID int64) error {
	_, err := store.NewUpdate().
		Model((*model.User)(nil)).
		Set("password_reset_issued_at = NULL").
		Where("id = ?", userID).
		Exec(ctx)
	return err
}

// format renders the secret as "<payload>.<signature>", both base64url so the
// whole thing survives a URL, a console and a copy-paste unchanged.
func format(userID int64, issuedAt time.Time, passwordHash, key string) string {
	return encode(payload(userID, issuedAt)) + "." + sign(userID, issuedAt, passwordHash, key)
}

// payload is the part of the secret the server reads back out of it.
func payload(userID int64, issuedAt time.Time) string {
	return strconv.FormatInt(userID, 10) + "." + strconv.FormatInt(issuedAt.Unix(), 10)
}

// sign covers the payload and the password hash in force when it was issued.
func sign(userID int64, issuedAt time.Time, passwordHash, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload(userID, issuedAt)))
	mac.Write([]byte{0})
	mac.Write([]byte(passwordHash))
	return encode(string(mac.Sum(nil)))
}

func parse(secret string) (int64, time.Time, string, error) {
	encodedPayload, signature, ok := strings.Cut(secret, ".")
	if !ok {
		return 0, time.Time{}, "", ErrInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return 0, time.Time{}, "", ErrInvalid
	}
	userIDText, issuedAtText, ok := strings.Cut(string(decoded), ".")
	if !ok {
		return 0, time.Time{}, "", ErrInvalid
	}
	userID, err := strconv.ParseInt(userIDText, 10, 64)
	if err != nil {
		return 0, time.Time{}, "", ErrInvalid
	}
	seconds, err := strconv.ParseInt(issuedAtText, 10, 64)
	if err != nil {
		return 0, time.Time{}, "", ErrInvalid
	}
	return userID, time.Unix(seconds, 0).UTC(), signature, nil
}

func encode(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
