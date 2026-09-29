package identity

import (
	"context"
	"net/http"
	"time"
)

// SessionCookieName is the cookie carrying the session. The __Host- prefix is
// enforced by the browser rather than by us: a cookie named this way is accepted
// only when it is Secure, has Path=/, and carries no Domain, which makes it
// impossible for a subdomain to write one.
const SessionCookieName = "__Host-sid"

// sessionCookie builds the cookie with the attributes the __Host- prefix
// requires. Only value and lifetime ever differ between setting and clearing it,
// so both go through here and cannot drift apart.
func sessionCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	}
}

// SetSessionCookie writes the session cookie with a lifetime of ttl. It is a
// no-op when there is no HTTP response to write it on — over a WebSocket the
// handshake is long gone, which is why the schema puts login on HTTP.
func SetSessionCookie(ctx context.Context, token string, ttl time.Duration) {
	writer, ok := ResponseWriterFromContext(ctx)
	if !ok {
		return
	}
	http.SetCookie(writer, sessionCookie(token, int(ttl.Seconds())))
}

// ClearSessionCookie expires the session cookie.
func ClearSessionCookie(ctx context.Context) {
	writer, ok := ResponseWriterFromContext(ctx)
	if !ok {
		return
	}
	http.SetCookie(writer, sessionCookie("", -1))
}

// SessionCookie returns the session token from the request cookie, if present.
func SessionCookie(request *http.Request) (string, bool) {
	cookie, err := request.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}
