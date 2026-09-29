// Package session manages the only credential the control server issues.
//
// A session token is nothing but a random secret; only sha256(secret) is stored
// and the session is found by that hash, so the sessions table cannot be
// replayed as a set of cookies and the cookie says nothing about the row it
// names. The token itself never
// changes: a session slides by moving its expiry, not by rotating its secret, so
// the cookie a browser holds stays valid for as long as the session does.
package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"control/internal/database"
	"control/internal/model"
	"control/internal/network"
)

const (
	// StepUpTTL is how long a session counts as password-proved. Short enough
	// that an unattended browser does not stay dangerous, long enough that a run
	// of administrative changes needs one password, not one per change.
	StepUpTTL = 30 * time.Minute

	// MaxLivePerUser caps how many live sessions one user may hold. Reaching it
	// is not an error: the least recently used session is ended to make room, so
	// signing in on a new device never fails, it just costs the oldest one.
	MaxLivePerUser = 10

	// slideDivisor places the sliding window: a session is extended by a full
	// term when it is used with less than a third of its term left. Extending on
	// every use would rewrite the row on every request for no added lifetime.
	slideDivisor = 3

	secretBytes = 32
)

// ErrInvalid is returned for a token that names no usable session — unknown,
// ended, or expired. They are deliberately one error: to the holder of a bad
// cookie they are all "sign in again".
var ErrInvalid = errors.New("invalid session")

// Origin describes where a session is being used from. It is recorded so the
// owner can recognise their own devices on the sessions screen; nothing
// authorizes on it.
type Origin struct {
	UserAgent string
	IPAddress string
}

// Issued is the result of creating a session: the token to hand to the client,
// the row that backs it, and the sessions that had to end for it to fit under
// the live cap.
type Issued struct {
	Token   string
	Session model.Session
	Evicted []int64
}

// Issue creates a session for userID with a term of ttl, ending the least
// recently used session if the user is already at the cap. steppedUp records
// whether the caller has just proved a password, which login and setPassword
// have and a returning cookie has not.
func Issue(ctx context.Context, store *database.Database, userID int64, ttl time.Duration, origin Origin, steppedUp bool) (Issued, error) {
	now := time.Now().UTC()
	evicted, err := enforceLiveLimit(ctx, store, userID, now)
	if err != nil {
		return Issued{}, err
	}

	secret, err := randomSecret()
	if err != nil {
		return Issued{}, err
	}
	session := &model.Session{
		UserID:        userID,
		SecretHash:    hashSecret(secret),
		TTLSeconds:    int64(ttl.Seconds()),
		LastUserAgent: origin.UserAgent,
		LastIPAddress: origin.IPAddress,
		ExpiresAt:     now.Add(ttl),
		CreatedAt:     now,
		LastUsedAt:    now,
	}
	if steppedUp {
		session.StepUpExpiresAt = now.Add(StepUpTTL)
	}

	if _, err := store.NewInsert().Model(session).Exec(ctx); err != nil {
		return Issued{}, err
	}
	return Issued{Token: secret, Session: *session, Evicted: evicted}, nil
}

// enforceLiveLimit ends the least recently used live sessions until one slot is
// free, and returns what it ended.
func enforceLiveLimit(ctx context.Context, store *database.Database, userID int64, now time.Time) ([]int64, error) {
	var live []int64
	if err := store.NewSelect().
		Model((*model.Session)(nil)).
		Column("id").
		Where("user_id = ?", userID).
		Where("ended_at IS NULL").
		Where("expires_at > ?", now).
		Order("last_used_at ASC", "id ASC").
		Scan(ctx, &live); err != nil {
		return nil, err
	}
	if len(live) < MaxLivePerUser {
		return nil, nil
	}

	evicted := live[:len(live)-MaxLivePerUser+1]
	for _, id := range evicted {
		if err := End(ctx, store, id); err != nil {
			return nil, err
		}
	}
	return evicted, nil
}

// Resolve verifies a token and returns the session it names, recording the use:
// lastUsedAt and the origin are updated, and a session used in the last third of
// its term is extended by a full term. slid reports that extension, which is
// what tells the caller to send the cookie again with a restarted Max-Age.
func Resolve(ctx context.Context, store *database.Database, token string, origin Origin) (session model.Session, slid bool, err error) {
	if token == "" {
		return model.Session{}, false, ErrInvalid
	}
	// Looking the hash up is not a timing oracle the way comparing the secret
	// would be: the caller controls the secret, not the hash it turns into.
	err = store.NewSelect().Model(&session).Where("secret_hash = ?", hashSecret(token)).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Session{}, false, ErrInvalid
	}
	if err != nil {
		return model.Session{}, false, err
	}

	now := time.Now().UTC()
	if !session.Live(now) {
		return model.Session{}, false, ErrInvalid
	}

	session.LastUsedAt = now
	session.LastUserAgent = origin.UserAgent
	session.LastIPAddress = origin.IPAddress
	if now.After(session.ExpiresAt.Add(-session.Term() / slideDivisor)) {
		session.ExpiresAt = now.Add(session.Term())
		slid = true
	}
	if _, err := store.NewUpdate().
		Model(&session).
		Column("last_used_at", "last_user_agent", "last_ip_address", "expires_at").
		WherePK().
		Exec(ctx); err != nil {
		return model.Session{}, false, err
	}
	return session, slid, nil
}

// Raise marks a session as having just proved a password.
func Raise(ctx context.Context, store *database.Database, sessionID int64) (time.Time, error) {
	stepUpExpiresAt := time.Now().UTC().Add(StepUpTTL)
	_, err := store.NewUpdate().
		Model((*model.Session)(nil)).
		Set("step_up_expires_at = ?", stepUpExpiresAt).
		Where("id = ?", sessionID).
		Exec(ctx)
	if err != nil {
		return time.Time{}, err
	}
	return stepUpExpiresAt, nil
}

// Get reads one session by id.
func Get(ctx context.Context, store *database.Database, sessionID int64) (model.Session, error) {
	var session model.Session
	err := store.NewSelect().Model(&session).Where("id = ?", sessionID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Session{}, ErrInvalid
	}
	if err != nil {
		return model.Session{}, err
	}
	return session, nil
}

// End closes a single session. Ending an already-ended one is a no-op, so the
// recorded time is the first decision rather than the last repetition of it.
func End(ctx context.Context, store *database.Database, sessionID int64) error {
	_, err := store.NewUpdate().
		Model((*model.Session)(nil)).
		Set("ended_at = ?", time.Now().UTC()).
		Where("id = ?", sessionID).
		Where("ended_at IS NULL").
		Exec(ctx)
	return err
}

// EndOthers closes every live session of a user except keepID, and returns what
// it closed. Passing 0 as keepID ends all of them, since no row has that id.
func EndOthers(ctx context.Context, store *database.Database, userID, keepID int64) ([]int64, error) {
	var ended []int64
	if err := store.NewSelect().
		Model((*model.Session)(nil)).
		Column("id").
		Where("user_id = ?", userID).
		Where("id != ?", keepID).
		Where("ended_at IS NULL").
		Where("expires_at > ?", time.Now().UTC()).
		Scan(ctx, &ended); err != nil {
		return nil, err
	}
	for _, id := range ended {
		if err := End(ctx, store, id); err != nil {
			return nil, err
		}
	}
	return ended, nil
}

// Sort is an ordering for a session list. It is this package's own enum rather
// than the schema's, so that nothing below the resolvers depends on the GraphQL
// layer.
type Sort int

const (
	SortCreatedDesc Sort = iota
	SortCreatedAsc
	SortLastUsedDesc
	SortLastUsedAsc
	SortEndedDesc
	SortEndedAsc
)

// orderBy renders a Sort as the arguments of an ORDER BY. Every ordering ends
// with the id so that equal timestamps still produce one stable order, which is
// what a client diffing a list needs.
func (s Sort) orderBy() []string {
	switch s {
	case SortCreatedAsc:
		return []string{"created_at ASC", "id ASC"}
	case SortLastUsedDesc:
		return []string{"last_used_at DESC", "id DESC"}
	case SortLastUsedAsc:
		return []string{"last_used_at ASC", "id ASC"}
	case SortEndedDesc:
		return []string{"ended_at DESC", "id DESC"}
	case SortEndedAsc:
		return []string{"ended_at ASC", "id ASC"}
	default:
		return []string{"created_at DESC", "id DESC"}
	}
}

// Filter narrows a session list. A nil field is not a filter.
type Filter struct {
	Live          *bool
	UserAgent     *string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	EndedAfter    *time.Time
	EndedBefore   *time.Time
}

// List returns a user's sessions, live and ended.
func List(ctx context.Context, store *database.Database, userID int64, filter Filter, sort Sort, limit int) ([]model.Session, error) {
	sessions := []model.Session{}
	query := store.NewSelect().Model(&sessions).Where("user_id = ?", userID)

	if filter.Live != nil {
		// "Live" is one condition with two halves, and the negation has to keep
		// them together: an ended session and an expired one are both not live.
		const live = "(ended_at IS NULL AND expires_at > ?)"
		if *filter.Live {
			query = query.Where(live, time.Now().UTC())
		} else {
			query = query.Where("NOT "+live, time.Now().UTC())
		}
	}
	if filter.UserAgent != nil {
		query = query.Where(`last_user_agent LIKE ? ESCAPE '\'`, "%"+escapeLike(*filter.UserAgent)+"%")
	}
	for _, bound := range []struct {
		condition string
		value     *time.Time
	}{
		{"created_at > ?", filter.CreatedAfter},
		{"created_at < ?", filter.CreatedBefore},
		{"ended_at > ?", filter.EndedAfter},
		{"ended_at < ?", filter.EndedBefore},
	} {
		if bound.value != nil {
			query = query.Where(bound.condition, *bound.value)
		}
	}

	if err := query.Order(sort.orderBy()...).Limit(limit).Scan(ctx); err != nil {
		return nil, err
	}
	return sessions, nil
}

// OriginFromRequest reads the caller's device and address off an HTTP request,
// the address being the one the rate limits count by too.
func OriginFromRequest(request *http.Request) Origin {
	return Origin{UserAgent: request.UserAgent(), IPAddress: network.ClientAddress(request)}
}

// escapeLike neutralises the wildcards in a user-supplied LIKE pattern, so a
// search for "%" means the character rather than "everything".
func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

func randomSecret() (string, error) {
	buffer := make([]byte, secretBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
