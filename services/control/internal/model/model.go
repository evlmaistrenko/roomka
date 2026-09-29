// Package model defines the database as Go types: one struct per table, and the
// relations between them. Everything else in the server reads and writes through
// these, and the migrations that create the tables are generated from them, so a
// column exists in exactly one place.
//
// Two conventions run through all of it:
//
//   - SQL is snake_case, Go and GraphQL stay camelCase, and the tags here are the
//     single place the two meet. The names are written out rather than left to
//     bun's default so that renaming a Go field cannot silently rename a column.
//     The database keeps its own convention because it is the one layer whose
//     identifiers other tools handle — psql, dumps, a future Postgres, where an
//     unquoted camelCase name folds to lower case and stops matching.
//   - A nullable timestamp is a plain time.Time tagged nullzero, so "never" is
//     the zero time rather than a pointer. The states this models — a password
//     never set, a session never raised, a session not yet ended — are all
//     "nothing has happened yet", which the zero value already says.
package model

import (
	"time"

	"github.com/uptrace/bun"
)

// All is every table, in an order that can be created front to back: a table
// never references one that comes after it.
func All() []any {
	return []any{
		(*User)(nil),
		(*UserRole)(nil),
		(*UserPermission)(nil),
		(*Session)(nil),
		(*UserPreference)(nil),
	}
}

// User is an account.
//
// PasswordHash is empty until somebody redeems a reset secret, which is the
// normal state of a freshly created account; PasswordSetAt answers "was this row
// ever used". PasswordResetIssuedAt is the only trace of a reset secret: the
// secret itself is never stored, it is re-derived from (id, that time, the
// password hash), so redeeming one invalidates it.
type User struct {
	bun.BaseModel `bun:"table:users,alias:user"`

	ID           int64  `bun:"id,pk,autoincrement"`
	Username     string `bun:"username,notnull,unique"`
	DisplayName  string `bun:"display_name,notnull"`
	PasswordHash string `bun:"password_hash,notnull"` // argon2id
	// Rank is who this user may act on: strictly lower ranks, and nobody else.
	Rank                  int       `bun:"rank,notnull"`
	PasswordSetAt         time.Time `bun:"password_set_at,nullzero"`
	PasswordResetIssuedAt time.Time `bun:"password_reset_issued_at,nullzero"`
	CreatedAt             time.Time `bun:"created_at,notnull"`
	UpdatedAt             time.Time `bun:"updated_at,notnull"`

	// Roles and Permissions are loaded on demand with Relation(), which is what
	// turns "the roles of everyone on this page" into one extra query rather
	// than one per user.
	Roles       []UserRole       `bun:"rel:has-many,join:id=user_id"`
	Permissions []UserPermission `bun:"rel:has-many,join:id=user_id"`
}

// UserRole is one role held by one user. The permissions a role carries are not
// stored anywhere: they are declared in the GraphQL schema by @grants and read
// from it at startup — see internal/rbac.
type UserRole struct {
	bun.BaseModel `bun:"table:user_roles,alias:user_role"`

	UserID int64  `bun:"user_id,pk"`
	Role   string `bun:"role,pk"` // a Role enum value
}

// UserPermission is one permission granted to a user outside any role.
type UserPermission struct {
	bun.BaseModel `bun:"table:user_permissions,alias:user_permission"`

	UserID     int64  `bun:"user_id,pk"`
	Permission string `bun:"permission,pk"` // a Permission enum value
}

// Session is the only credential the server issues. The cookie carries a random
// secret and nothing else; only sha256(secret) is stored, and the row is found
// by it, so the table cannot be replayed as a set of cookies.
//
// TTLSeconds is the session's own term, chosen by the client at login: ExpiresAt
// slides forward by a full term whenever the session is used in its last third,
// which needs the term itself and not just the end of it.
//
// EndedAt and an ExpiresAt in the past are both "no longer usable", and they
// stay distinct because only one of them is a decision somebody made.
type Session struct {
	bun.BaseModel `bun:"table:sessions,alias:session"`

	ID              int64     `bun:"id,pk,autoincrement"`
	UserID          int64     `bun:"user_id,notnull"`
	SecretHash      string    `bun:"secret_hash,notnull,unique"`
	TTLSeconds      int64     `bun:"ttl_seconds,notnull"`
	LastUserAgent   string    `bun:"last_user_agent,nullzero"`
	LastIPAddress   string    `bun:"last_ip_address,nullzero"`
	ExpiresAt       time.Time `bun:"expires_at,notnull"`
	StepUpExpiresAt time.Time `bun:"step_up_expires_at,nullzero"`
	EndedAt         time.Time `bun:"ended_at,nullzero"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	LastUsedAt      time.Time `bun:"last_used_at,notnull"`
}

// Live reports whether a session can still be used.
func (s Session) Live(now time.Time) bool {
	return s.EndedAt.IsZero() && now.Before(s.ExpiresAt)
}

// Term is the session's own length, as chosen when it was issued. It is what a
// slide extends by, and what the cookie's Max-Age is set from.
func (s Session) Term() time.Duration {
	return time.Duration(s.TTLSeconds) * time.Second
}

// UserPreference is one client-defined key/value pair, opaque to the server.
// The key column is called name because `key` is a SQLite keyword and would have
// to be quoted everywhere it appeared.
type UserPreference struct {
	bun.BaseModel `bun:"table:user_preferences,alias:user_preference"`

	UserID int64  `bun:"user_id,pk"`
	Name   string `bun:"name,pk"`
	Value  string `bun:"value,notnull"`
}
