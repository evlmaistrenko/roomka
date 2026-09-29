package identity

import (
	"context"
	"net/http"
	"time"
)

type contextKey int

const (
	principalKey contextKey = iota
	responseWriterKey
	requestKey
)

// Principal identifies the authenticated caller and the session that proves it.
// A session is the only credential the server issues, so identity and credential
// are one thing here; the caller's permissions are deliberately not part of it
// and are read live from the database on every check.
type Principal struct {
	UserID    int64
	SessionID int64
	// StepUpExpiresAt is when the session stops counting as recently
	// password-proved. Zero when the session was never raised.
	StepUpExpiresAt time.Time
}

// SteppedUp reports whether the session still counts as password-proved.
func (p Principal) SteppedUp(now time.Time) bool {
	return !p.StepUpExpiresAt.IsZero() && now.Before(p.StepUpExpiresAt)
}

// WithPrincipal returns ctx carrying the authenticated principal.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey, principal)
}

// WithoutPrincipal returns ctx carrying no principal, whatever ctx carried
// before: the caller is anonymous from here on.
func WithoutPrincipal(ctx context.Context) context.Context {
	return context.WithValue(ctx, principalKey, nil)
}

// PrincipalFromContext returns the authenticated principal, if any.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey).(Principal)
	return principal, ok
}

// WithHTTP stashes the current request's writer and request in ctx so resolvers
// can read and set the session cookie — gqlgen does not pass these through.
func WithHTTP(ctx context.Context, writer http.ResponseWriter, request *http.Request) context.Context {
	ctx = context.WithValue(ctx, responseWriterKey, writer)
	return context.WithValue(ctx, requestKey, request)
}

// ResponseWriterFromContext returns the active HTTP response writer, if any.
func ResponseWriterFromContext(ctx context.Context) (http.ResponseWriter, bool) {
	writer, ok := ctx.Value(responseWriterKey).(http.ResponseWriter)
	return writer, ok
}

// RequestFromContext returns the active HTTP request, if any.
func RequestFromContext(ctx context.Context) (*http.Request, bool) {
	request, ok := ctx.Value(requestKey).(*http.Request)
	return request, ok
}
