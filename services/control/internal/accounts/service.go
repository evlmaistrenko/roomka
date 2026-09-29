// Package accounts is everything the current schema is about: who somebody is,
// how they prove it, and what they may do to whom. Signing in, passwords,
// profiles, sessions, preferences, the user list and its management all live
// here, because they are one subject rather than six.
//
// The GraphQL layer above it is a delegation shell: resolvers call these methods
// and add nothing. That split is the point of the package — a rule this side of
// the boundary cannot be quietly reached around, and the schema's own concerns
// (directives, the executable schema) cannot leak in.
//
// The service speaks the schema's vocabulary — it returns schema.User rather
// than a type of its own. With one API in front of it, a second set of domain
// types would be a translation layer with nothing on the other side of it.
package accounts

import (
	"context"
	"time"

	"control/internal/config"
	"control/internal/database"
	"control/internal/events"
	"control/internal/identity"
	"control/internal/model"
	"control/internal/rbac"
	"control/internal/schema"
)

// Service answers everything the accounts half of the schema asks. Its
// dependencies are unexported: what this package needs to do its work is not
// part of what it offers.
type Service struct {
	config   config.Config
	database *database.Database
	events   *events.Bus
	rbac     *rbac.Roles
	limits   *limits
}

// New builds the service.
func New(configuration config.Config, store *database.Database, bus *events.Bus, roles *rbac.Roles) *Service {
	return &Service{
		config:   configuration,
		database: store,
		events:   bus,
		rbac:     roles,
		limits:   newLimits(configuration.Limits),
	}
}

// maxPreferencesPerUser caps how many preference keys one user may store. The
// cap exists because preferences are the one place a client writes arbitrary
// data, so without one a client bug becomes unbounded storage.
const maxPreferencesPerUser = 100

// rootRank is the rank the first user is created at. Every other rank is capped
// at 99 by the schema, so nobody ever reaches it — which is what makes the root
// account impossible to demote, delete or reset, with no rule of its own.
const rootRank = 100

// done is the value returned by a field typed Void. Fields typed Void are
// nullable, so nil would serialize identically; returning a value keeps
// "succeeded" and "did nothing" distinguishable in the Go code.
var done = &schema.Void{}

// nowUTC is the single source of "now" for stored timestamps, which are all UTC.
func nowUTC() time.Time { return time.Now().UTC() }

// Version is the server's own version, as the schema reports it.
func (s *Service) Version() string { return s.config.Version }

// SetupRequired reports whether this server is still unclaimed.
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	claimed, err := s.database.NewSelect().Model((*model.User)(nil)).Exists(ctx)
	return !claimed, err
}

// Me answers "who am I", or nothing at all when nobody is signed in — which is
// an answer rather than a failure, and the one a client asks for before it has
// anything to show.
func (s *Service) Me(ctx context.Context) (*schema.Viewer, error) {
	principal, ok := identity.PrincipalFromContext(ctx)
	if !ok {
		return nil, nil
	}
	return s.viewer(ctx, principal)
}

// HasPermission reports whether a user holds a permission, directly or through a
// role. The @auth directive asks it: permissions belong to accounts, and the
// directive is only the place the question gets asked.
func (s *Service) HasPermission(ctx context.Context, userID int64, permission schema.Permission) (bool, error) {
	return s.rbac.Has(ctx, userID, string(permission))
}

// caller returns the authenticated principal. Every method that calls it sits
// behind a field carrying @auth, which has already turned away the
// unauthenticated, so the failure path here means the schema and this package
// have drifted apart rather than that a caller did something.
func caller(ctx context.Context) (identity.Principal, error) {
	principal, ok := identity.PrincipalFromContext(ctx)
	if !ok {
		return identity.Principal{}, schema.ErrUnauthenticated
	}
	return principal, nil
}

// What the service announces. Every change that a subscription shows is
// published here, and subscriptions.go consumes it; keeping the publishing in
// one place is what makes it possible to answer "what does this change notify"
// by reading rather than by grepping for topic names.

// publishViewer says that a user's own view of themselves changed.
func (s *Service) publishViewer(userID int64) {
	s.events.Publish(events.ViewerTopic(userID), nil)
}

// publishSessions says that a user's session list changed. The viewer goes with
// it because the viewer carries the current session.
func (s *Service) publishSessions(userID int64) {
	s.events.Publish(events.SessionsTopic(userID), nil)
	s.publishViewer(userID)
}

// publishUsers says that the set of users, or something about one of them,
// changed.
func (s *Service) publishUsers() {
	s.events.Publish(events.UsersTopic(), nil)
}

// publishAccessChanged says that what a user may do changed. Per the schema this
// ends every subscription that user has open, on every session: a stream that
// kept running would be a stream whose access was decided when it started.
func (s *Service) publishAccessChanged(userID int64) {
	s.events.Publish(events.AccessTopic(userID), nil)
	s.publishUsers()
}

// publishSessionEnded says that one session is over, which ends the
// subscriptions running on it and changes its owner's session list.
func (s *Service) publishSessionEnded(userID, sessionID int64) {
	s.events.Publish(events.SessionTopic(sessionID), nil)
	s.publishSessions(userID)
}
