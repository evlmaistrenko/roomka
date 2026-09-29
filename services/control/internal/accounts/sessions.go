package accounts

import (
	"context"
	"errors"

	"control/internal/identity"
	"control/internal/schema"
	"control/internal/session"
)

// defaultSessionLimit is the limit the schema declares on the sessions field. It
// is repeated here for the mutations that answer with a list but take no limit
// argument, so every session list a client sees is cut the same way.
const defaultSessionLimit = 50

// sessions lists the caller's own sessions, live and ended. There is no field
// anywhere that lists somebody else's: a session list is a list of a person's
// devices, and no permission in this schema grants a view of those.
func (s *Service) Sessions(ctx context.Context, filter *schema.SessionFilterInput, sortBy *schema.SessionSort, limit int) ([]*schema.Session, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	return s.listSessions(ctx, principal, toSessionFilter(filter), toSessionSort(sortBy), limit)
}

func (s *Service) listSessions(ctx context.Context, principal identity.Principal, filter session.Filter, sort session.Sort, limit int) ([]*schema.Session, error) {
	stored, err := session.List(ctx, s.database, principal.UserID, filter, sort, limit)
	if err != nil {
		return nil, err
	}
	return toSessions(stored, principal.SessionID), nil
}

// revokeSession ends one of the caller's other sessions.
func (s *Service) RevokeSession(ctx context.Context, sessionID string) ([]*schema.Session, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	id, ok := parseID(sessionID)
	if !ok {
		return nil, errSessionMissing
	}
	target, err := session.Get(ctx, s.database, id)
	if errors.Is(err, session.ErrInvalid) {
		return nil, errSessionMissing
	}
	if err != nil {
		return nil, err
	}
	// Somebody else's session is reported as missing rather than denied: the
	// answer must not confirm that a session id belongs to an account.
	if target.UserID != principal.UserID || !target.Live(nowUTC()) {
		return nil, errSessionMissing
	}
	if target.ID == principal.SessionID {
		return nil, schema.InvalidInput(schema.InvalidInputReasonMalformed, []string{"sessionId"},
			"the current session cannot be revoked; use logout")
	}

	if err := session.End(ctx, s.database, target.ID); err != nil {
		return nil, err
	}
	s.publishSessionEnded(principal.UserID, target.ID)
	return s.listSessions(ctx, principal, session.Filter{}, session.SortCreatedDesc, defaultSessionLimit)
}

// revokeOtherSessions ends every session but the one asking.
func (s *Service) RevokeOtherSessions(ctx context.Context) ([]*schema.Session, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	ended, err := session.EndOthers(ctx, s.database, principal.UserID, principal.SessionID)
	if err != nil {
		return nil, err
	}
	for _, sessionID := range ended {
		s.publishSessionEnded(principal.UserID, sessionID)
	}
	return s.listSessions(ctx, principal, session.Filter{}, session.SortCreatedDesc, defaultSessionLimit)
}

// toSessionFilter translates the schema's filter into the store's. The two are
// the same shape today and are still kept apart, so that the session package
// stays usable without the GraphQL types and a schema change does not reach into
// the database layer by itself.
func toSessionFilter(filter *schema.SessionFilterInput) session.Filter {
	if filter == nil {
		return session.Filter{}
	}
	return session.Filter{
		Live:          filter.Live,
		UserAgent:     filter.UserAgent,
		CreatedAfter:  filter.CreatedAfter,
		CreatedBefore: filter.CreatedBefore,
		EndedAfter:    filter.EndedAfter,
		EndedBefore:   filter.EndedBefore,
	}
}

func toSessionSort(sortBy *schema.SessionSort) session.Sort {
	if sortBy == nil {
		return session.SortCreatedDesc
	}
	switch *sortBy {
	case schema.SessionSortCreatedAsc:
		return session.SortCreatedAsc
	case schema.SessionSortLastUsedDesc:
		return session.SortLastUsedDesc
	case schema.SessionSortLastUsedAsc:
		return session.SortLastUsedAsc
	case schema.SessionSortEndedDesc:
		return session.SortEndedDesc
	case schema.SessionSortEndedAsc:
		return session.SortEndedAsc
	default:
		return session.SortCreatedDesc
	}
}

// errSessionMissing answers every session id that names nothing the caller may
// end — unknown, already ended, or somebody else's.
var errSessionMissing = schema.InvalidInput(schema.InvalidInputReasonTargetMissing, []string{"sessionId"},
	"no such session")
