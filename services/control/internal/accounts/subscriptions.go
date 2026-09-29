package accounts

import (
	"context"
	"time"

	"control/internal/events"
	"control/internal/identity"
	"control/internal/schema"
	"control/internal/session"
)

// Every subscription here has the same shape, and the shape is the schema's
// promise: emit the state at subscribe time, emit it again on every change, and
// end — without an error — as soon as the access that allowed it changes.
//
// Ending rather than re-checking is deliberate. A long-lived stream that kept
// re-evaluating permission would be a second access-control path, live in a
// different place from @auth and easy to get subtly wrong; ending sends the
// client back through the same door it came in by.

// subscribeViewer streams "who am I", which changes when the profile, the
// permissions, the preferences or the session behind it change.
func (s *Service) SubscribeViewer(ctx context.Context) (<-chan *schema.Viewer, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	return subscribe(ctx, s, principal, events.ViewerTopic(principal.UserID),
		func(ctx context.Context) (*schema.Viewer, error) { return s.viewer(ctx, principal) })
}

// subscribeSessions streams the caller's session list, which changes when they
// sign in elsewhere, revoke something, or a session is ended for them.
func (s *Service) SubscribeSessions(ctx context.Context, filter *schema.SessionFilterInput, sortBy *schema.SessionSort, limit int) (<-chan []*schema.Session, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	sessionFilter, sort := toSessionFilter(filter), toSessionSort(sortBy)
	return subscribe(ctx, s, principal, events.SessionsTopic(principal.UserID),
		func(ctx context.Context) ([]*schema.Session, error) {
			return s.listSessions(ctx, principal, sessionFilter, sort, limit)
		})
}

// subscribeUsersChanged is a bell, not a feed: it carries no payload, and a
// client answers it by refetching the page it is showing with the filter and
// sort it is showing them under. Sending the list instead would mean the server
// guessing which page of which query each watcher is on.
func (s *Service) SubscribeUsersChanged(ctx context.Context) (<-chan *schema.Void, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	return subscribe(ctx, s, principal, events.UsersTopic(),
		func(context.Context) (*schema.Void, error) { return done, nil })
}

// subscribe is the shape all three share: build the payload once up front — so
// that a subscription which cannot be served fails at subscribe time, where the
// client can still be told why — then rebuild it on every event until the
// subscription ends.
func subscribe[T any](ctx context.Context, service *Service, principal identity.Principal, topic string, build func(context.Context) (T, error)) (<-chan T, error) {
	first, err := build(ctx)
	if err != nil {
		return nil, err
	}

	updates := service.events.Subscribe(ctx, topic)
	ending := service.ending(ctx, principal)

	stream := make(chan T, 1)
	stream <- first
	go func() {
		defer close(stream)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ending:
				return
			case _, open := <-updates:
				if !open {
					return
				}
				// An ending and an update can be published by the same mutation
				// — revoking a session both ends it and changes the list. The
				// ending wins whenever it is already known.
				select {
				case <-ending:
					return
				default:
				}
				value, err := build(ctx)
				if err != nil {
					// Nothing can carry an error to the client at this point;
					// ending the stream is the one signal available, and it is
					// the signal the client already knows how to handle.
					return
				}
				select {
				case stream <- value:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return stream, nil
}

// ending returns a channel that closes when the subscription must stop: the
// caller's access changed, their session ended, or the session ran out of time
// while the socket stayed open.
func (s *Service) ending(ctx context.Context, principal identity.Principal) <-chan struct{} {
	access := s.events.Subscribe(ctx, events.AccessTopic(principal.UserID))
	ended := s.events.Subscribe(ctx, events.SessionTopic(principal.SessionID))

	stop := make(chan struct{})
	go func() {
		defer close(stop)
		// A session with no HTTP traffic never slides, but one with a browser
		// tab open alongside the socket does, so the deadline is re-read rather
		// than trusted once: the timer firing asks the question again.
		expiry := time.NewTimer(s.untilExpiry(ctx, principal.SessionID))
		defer expiry.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-access:
				return
			case <-ended:
				return
			case <-expiry.C:
				remaining := s.untilExpiry(ctx, principal.SessionID)
				if remaining <= 0 {
					return
				}
				expiry.Reset(remaining)
			}
		}
	}()
	return stop
}

// untilExpiry is how long the session has left, or zero once it has none. A
// session that cannot be read at all has none either: the row is gone.
func (s *Service) untilExpiry(ctx context.Context, sessionID int64) time.Duration {
	current, err := session.Get(ctx, s.database, sessionID)
	if err != nil {
		return 0
	}
	return time.Until(current.ExpiresAt)
}
