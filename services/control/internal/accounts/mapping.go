package accounts

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"time"

	"control/internal/identity"
	"control/internal/model"
	"control/internal/schema"
	"control/internal/session"
)

// formatID renders a database row id as a GraphQL ID.
func formatID(id int64) string { return strconv.FormatInt(id, 10) }

// parseID reads a GraphQL ID back into a row id. A malformed id is not reported
// as malformed: an id that cannot exist and an id that does not exist are the
// same answer, and giving them one shape keeps clients off the difference.
func parseID(id string) (int64, bool) {
	parsed, err := strconv.ParseInt(id, 10, 64)
	return parsed, err == nil
}

// isNoRows distinguishes "no such row" from a real failure.
func isNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }

// viewer assembles the answer to "who am I": the account, what this session may
// do, the session itself, and the client's stored preferences.
func (s *Service) viewer(ctx context.Context, principal identity.Principal) (*schema.Viewer, error) {
	user, err := s.loadUser(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}
	current, err := session.Get(ctx, s.database, principal.SessionID)
	if err != nil {
		return nil, err
	}
	preferences, err := s.loadPreferences(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}
	return &schema.Viewer{
		User: user,
		// A session carries the whole of its user's rights: there is no
		// credential here that holds less than its owner does. The field stays
		// separate from User.permissions because that stops being true the
		// moment a narrower credential exists, and clients should already be
		// gating on the session rather than on the account.
		Permissions: user.Permissions,
		Session:     toSession(current, principal.SessionID),
		Preferences: preferences,
	}, nil
}

// loadUser reads one user with the roles and permissions held, in one call.
func (s *Service) loadUser(ctx context.Context, userID int64) (*schema.User, error) {
	var stored model.User
	if err := s.database.NewSelect().
		Model(&stored).
		Relation("Roles").
		Relation("Permissions").
		Where("user.id = ?", userID).
		Scan(ctx); err != nil {
		return nil, err
	}
	return s.toUser(stored), nil
}

// toUser renders a stored user for the schema, resolving the three permission
// views it exposes: the roles held, the permissions granted directly, and the
// union of both. Roles carry their permissions from the GraphQL schema, not from
// the database, so the union is computed here rather than read.
func (s *Service) toUser(stored model.User) *schema.User {
	roleNames := make([]string, 0, len(stored.Roles))
	for _, role := range stored.Roles {
		roleNames = append(roleNames, role.Role)
	}
	directNames := make([]string, 0, len(stored.Permissions))
	for _, permission := range stored.Permissions {
		directNames = append(directNames, permission.Permission)
	}
	slices.Sort(roleNames)
	slices.Sort(directNames)

	user := &schema.User{
		ID:                formatID(stored.ID),
		Username:          stored.Username,
		DisplayName:       stored.DisplayName,
		Rank:              stored.Rank,
		Roles:             toRoles(roleNames),
		DirectPermissions: toPermissions(directNames),
		Permissions:       s.effectivePermissions(roleNames, directNames),
		CreatedAt:         stored.CreatedAt,
	}
	if !stored.PasswordSetAt.IsZero() {
		passwordSetAt := stored.PasswordSetAt
		user.PasswordSetAt = &passwordSetAt
	}
	return user
}

// effectivePermissions unions what a user holds directly with what their roles
// grant.
func (s *Service) effectivePermissions(roleNames, directNames []string) []schema.Permission {
	names := slices.Clone(directNames)
	for _, role := range roleNames {
		for _, granted := range s.rbac.Granted(role) {
			if !slices.Contains(names, granted) {
				names = append(names, granted)
			}
		}
	}
	slices.Sort(names)
	return toPermissions(names)
}

func toRoles(names []string) []schema.Role {
	roles := make([]schema.Role, 0, len(names))
	for _, name := range names {
		roles = append(roles, schema.Role(name))
	}
	return roles
}

func toPermissions(names []string) []schema.Permission {
	permissions := make([]schema.Permission, 0, len(names))
	for _, name := range names {
		permissions = append(permissions, schema.Permission(name))
	}
	return permissions
}

// toSession renders a stored session for the schema. currentID is the session
// the request is being made on, which is the only thing that makes one row in
// the list different from the others.
func toSession(stored model.Session, currentID int64) *schema.Session {
	rendered := &schema.Session{
		ID:         formatID(stored.ID),
		Current:    stored.ID == currentID,
		CreatedAt:  stored.CreatedAt,
		LastUsedAt: stored.LastUsedAt,
		ExpiresAt:  stored.ExpiresAt,
	}
	if stored.LastUserAgent != "" {
		rendered.LastUserAgent = &stored.LastUserAgent
	}
	if stored.LastIPAddress != "" {
		rendered.LastIPAddress = &stored.LastIPAddress
	}
	if !stored.StepUpExpiresAt.IsZero() {
		rendered.StepUpExpiresAt = &stored.StepUpExpiresAt
	}
	if !stored.EndedAt.IsZero() {
		rendered.EndedAt = &stored.EndedAt
	}
	return rendered
}

func toSessions(stored []model.Session, currentID int64) []*schema.Session {
	sessions := make([]*schema.Session, 0, len(stored))
	for _, one := range stored {
		sessions = append(sessions, toSession(one, currentID))
	}
	return sessions
}

// requestOrigin describes the caller's device for the sessions screen. Both
// values are absent when there is no HTTP request to read them from.
func requestOrigin(ctx context.Context) session.Origin {
	request, ok := identity.RequestFromContext(ctx)
	if !ok {
		return session.Origin{}
	}
	return session.OriginFromRequest(request)
}

// startSession issues a session for userID and puts its token in the cookie.
// The session starts raised because every path that creates one — setup, login,
// setPassword — has just seen a password proved.
func (s *Service) startSession(ctx context.Context, userID int64, ttl time.Duration) (model.Session, error) {
	issued, err := session.Issue(ctx, s.database, userID, ttl, requestOrigin(ctx), true)
	if err != nil {
		return model.Session{}, err
	}
	identity.SetSessionCookie(ctx, issued.Token, ttl)
	for _, ended := range issued.Evicted {
		s.publishSessionEnded(userID, ended)
	}
	return issued.Session, nil
}

// sessionTTL turns the client's chosen term into a duration. The schema's
// @constraint has already bounded it.
func sessionTTL(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}
