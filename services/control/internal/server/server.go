// Package server is the control server's HTTP surface: the routes, the GraphQL
// handler, and the middleware that turns a session cookie into an authenticated
// caller. It is a package rather than part of main so that a test can run the
// real server — the same transports, the same middleware, the same error
// shapes — instead of a hand-built approximation of it.
package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/vektah/gqlparser/v2/ast"

	"control/graph"
	"control/internal/config"
	"control/internal/database"
	"control/internal/identity"
	"control/internal/ratelimit"
	"control/internal/session"
	"control/rest"
)

const (
	// complexityLimit caps the total complexity of a single operation so a
	// deeply nested or wide query cannot exhaust the server.
	complexityLimit = 200

	// websocketCloseSessionInvalid is the close code the schema names for a
	// socket whose session is gone. It is in the private 4000-4999 range, where
	// the application — not the WebSocket protocol — decides what a code means.
	websocketCloseSessionInvalid = 4403
)

// New builds the whole HTTP handler: /graphql, the REST endpoints the OpenAPI
// spec declares, and the tools for browsing both. It takes the store and the
// configuration in their own right rather than off the resolver, which carries
// nothing but the service it delegates to.
//
// The server behaves the same in development and production. Which of these
// routes the outside world reaches is the reverse proxy's call: the tools are
// plain routes it can simply not forward.
func New(configuration config.Config, store *database.Database, resolver *graph.Resolver) (http.Handler, error) {
	policy := configuration.Limits
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	graphqlServer := newGraphQLServer(configuration, store, resolver)

	mux := http.NewServeMux()
	mux.Handle("/graphql", withSocketLimit(policy,
		withRequestContext(store, configuration.PublicURL, graphqlServer)))
	rest.HandlerFromMux(rest.NewStrictHandler(rest.Server{}, nil), mux)

	// Apollo Sandbox over /graphql, and Swagger UI over the OpenAPI spec. Both
	// run against this same origin.
	mux.Handle("GET /graphql/playground", playground.ApolloSandboxHandler("roomka control", "/graphql"))
	spec, err := rest.SpecHandler(configuration.Version)
	if err != nil {
		return nil, fmt.Errorf("loading the embedded OpenAPI spec: %w", err)
	}
	mux.Handle("GET /openapi.json", spec)
	docs := rest.DocsHandler("/openapi/", "/openapi.json")
	mux.Handle("GET /openapi", docs)
	mux.Handle("GET /openapi/", docs)

	// The health check is exempt from the request limits: whatever polls it
	// sits next to the server, and a probe refused for polling is a healthy
	// server reported dead.
	limited := withRequestLimits(policy, mux)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			mux.ServeHTTP(writer, request)
			return
		}
		limited.ServeHTTP(writer, request)
	}), nil
}

// newGraphQLServer builds the gqlgen handler: HTTP for queries and mutations, a
// WebSocket for subscriptions, and the schema's directives.
func newGraphQLServer(configuration config.Config, store *database.Database, resolver *graph.Resolver) *handler.Server {
	executableSchema := graph.NewExecutableSchema(graph.Config{
		Resolvers:  resolver,
		Directives: resolver.Directives(),
	})

	server := handler.New(executableSchema)
	server.AddTransport(transport.Options{})
	server.AddTransport(transport.GET{})
	server.AddTransport(transport.POST{})
	socketMessageBytes := int64(maxSocketMessageBytes)
	server.AddTransport(transport.Websocket{
		// KeepAlivePingInterval only covers the legacy graphql-ws subprotocol.
		// On graphql-transport-ws, the one the schema names, gqlgen sends
		// nothing unless PingPongInterval is set, and a socket carrying no
		// events is then silent for as long as it lives: whatever sits in the
		// path with an idle timeout (a proxy, a load balancer, an antivirus
		// inspecting HTTP) cuts it, and a peer that vanished is never noticed.
		// The client answers each ping with a pong; one that misses two in a
		// row is dropped.
		PingPongInterval:      socketPingInterval,
		KeepAlivePingInterval: socketPingInterval,
		InitTimeout:           socketInitTimeout,
		PayloadReadLimit:      &socketMessageBytes,
		InitFunc:              websocketInit(store, configuration.Limits),
	})
	server.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	// Introspection is on by default: the SDL is public in the repository
	// anyway, so hiding it from the live server would protect nothing and only
	// cost the tools that read a schema off the endpoint. The switch is for a
	// deployment whose schema is not public. It lives here, after parsing,
	// because only here is it closed on every transport at once.
	if !configuration.DisableGraphQLIntrospection {
		server.Use(extension.Introspection{})
	}
	server.Use(extension.FixedComplexityLimit(complexityLimit))
	server.Use(socketLimits{})
	server.Use(socketSession{store: store})
	server.Use(extension.AutomaticPersistedQuery{Cache: lru.New[string](100)})
	server.SetErrorPresenter(errorPresenter)
	return server
}

// websocketInit decides what happens to a socket at the moment it opens, and for
// as long as it stays open.
//
// Nothing authenticates inside the socket: the principal is already in the
// context, put there by the middleware from the cookie the browser sent on the
// handshake. What is left is the two things a long-lived connection has to do
// that a request does not — refuse to open on a dead session, and not outlive a
// session that dies under it. Every socket that opens also gets its own limits,
// which its operations find in the context this returns.
func websocketInit(store *database.Database, policy ratelimit.Policy) transport.WebsocketInitFunc {
	return func(ctx context.Context, initPayload transport.InitPayload) (context.Context, *transport.InitPayload, error) {
		ctx = withSocketState(ctx, policy)
		principal, ok := identity.PrincipalFromContext(ctx)
		if !ok {
			if _, presented := sessionCookieOf(ctx); presented {
				// A cookie that no longer resolves: say so with the code the
				// schema names, rather than leaving the client to discover it
				// one failed subscribe at a time.
				return transport.WithWebsocketCloseCode(ctx, websocketCloseSessionInvalid),
					&initPayload, errors.New("session is no longer valid")
			}
			// No cookie at all is allowed to connect. Every subscription field
			// carries @auth, so the connection can do nothing until it has one.
			return ctx, &initPayload, nil
		}

		ctx, cancel := context.WithCancel(transport.AppendCloseReason(ctx, "session expired"))
		go closeOnExpiry(ctx, cancel, store, principal.SessionID)
		return ctx, &initPayload, nil
	}
}

// closeOnExpiry cancels the connection's context when its session runs out,
// which closes the socket.
//
// The deadline is re-read rather than trusted once: a browser with a tab open
// alongside the socket keeps sliding the session over HTTP, and the socket has
// no way to see that happen.
func closeOnExpiry(ctx context.Context, cancel context.CancelFunc, store *database.Database, sessionID int64) {
	defer cancel()
	for {
		current, err := session.Get(ctx, store, sessionID)
		if err != nil {
			return
		}
		remaining := time.Until(current.ExpiresAt)
		if remaining <= 0 {
			return
		}
		timer := time.NewTimer(remaining)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// socketSession brings a socket's principal up to date at the start of every
// operation on it.
//
// The principal is resolved once, from the cookie on the handshake, but the
// socket outlives any single operation and may outlive the session too: a
// logout or a revocation ends the subscriptions running at that moment, and
// the client then subscribes again on the same socket. Without this the new
// subscription would be served on the dead session's authority. With it, the
// operation runs as anonymous and fails the way a request with a dead cookie
// does. The session row is re-read for step-up too, which an HTTP request can
// raise while the socket stays open.
type socketSession struct {
	store *database.Database
}

var (
	_ graphql.HandlerExtension     = socketSession{}
	_ graphql.OperationInterceptor = socketSession{}
)

func (socketSession) ExtensionName() string                   { return "SocketSession" }
func (socketSession) Validate(graphql.ExecutableSchema) error { return nil }

func (s socketSession) InterceptOperation(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
	if _, onSocket := ctx.Value(socketStateKey{}).(*socketState); !onSocket {
		// A request resolved its cookie a moment ago, in withRequestContext.
		return next(ctx)
	}
	principal, ok := identity.PrincipalFromContext(ctx)
	if !ok {
		return next(ctx)
	}
	current, err := session.Get(ctx, s.store, principal.SessionID)
	if err != nil && !errors.Is(err, session.ErrInvalid) {
		return refuseOperation(ctx, err)
	}
	if err != nil || !current.Live(time.Now().UTC()) {
		return next(identity.WithoutPrincipal(ctx))
	}
	principal.StepUpExpiresAt = current.StepUpExpiresAt
	return next(identity.WithPrincipal(ctx, principal))
}

// withRequestContext makes the HTTP writer and request available to resolvers
// (which need them for the session cookie) and resolves that cookie into an
// authenticated principal.
func withRequestContext(store *database.Database, publicURL string, next http.Handler) http.Handler {
	allowedOrigin := originOf(publicURL)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx := identity.WithHTTP(request.Context(), writer, request)

		token, presented := identity.SessionCookie(request)
		if presented && !originAllowed(request, allowedOrigin) {
			// The cookie is SameSite=Strict, so this is a second lock on the
			// same door — and the one that still holds if a browser ever sends
			// the cookie on a cross-site request anyway.
			http.Error(writer, "origin not allowed", http.StatusForbidden)
			return
		}
		if presented {
			ctx = authenticate(ctx, store, request, token)
		}
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

// authenticate resolves the session cookie, records the use, and re-sends the
// cookie whenever the session slid — which is what keeps a cookie's Max-Age in
// step with the expiry it stands for.
func authenticate(ctx context.Context, store *database.Database, request *http.Request, token string) context.Context {
	current, slid, err := session.Resolve(ctx, store, token, session.OriginFromRequest(request))
	if errors.Is(err, session.ErrInvalid) {
		// The browser is holding a cookie that will never work again. Clearing
		// it stops every subsequent request from carrying it.
		identity.ClearSessionCookie(ctx)
		return ctx
	}
	if err != nil {
		log.Printf("resolving session: %v", err)
		return ctx
	}
	if slid {
		identity.SetSessionCookie(ctx, token, current.Term())
	}
	return identity.WithPrincipal(ctx, identity.Principal{
		UserID:          current.UserID,
		SessionID:       current.ID,
		StepUpExpiresAt: current.StepUpExpiresAt,
	})
}

// sessionCookieOf reports whether the request behind ctx carried a session
// cookie. It answers "was a credential offered", which is a different question
// from "is there a principal".
func sessionCookieOf(ctx context.Context) (string, bool) {
	request, ok := identity.RequestFromContext(ctx)
	if !ok {
		return "", false
	}
	return identity.SessionCookie(request)
}

// originAllowed applies the schema's rule that Origin is checked on every call
// that carries the cookie. Two origins pass: the app this server exists for, and
// this server itself.
//
// The second is not a loophole. A page can only produce an Origin equal to the
// one it was addressed at if this server served that page, because the browser
// writes the header and a page cannot forge it — which is the whole basis of the
// check. It is also the only way the built-in playground can work, since it is
// served from the API's own origin rather than the UI's.
//
// A request with no Origin at all is not a browser request — nothing else sets
// the header — so it is left to the cookie itself.
func originAllowed(request *http.Request, allowedOrigin string) bool {
	origin := request.Header.Get("Origin")
	return origin == "" || origin == allowedOrigin || origin == ownOrigin(request)
}

// ownOrigin is the origin the request was addressed to. Behind the reverse proxy
// the connection this server sees is plain HTTP, so the scheme comes from the
// forwarded header when there is one.
func ownOrigin(request *http.Request) string {
	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	}
	if forwarded := request.Header.Get("X-Forwarded-Proto"); forwarded != "" {
		scheme = forwarded
	}
	return scheme + "://" + request.Host
}

// originOf reduces a URL to the scheme and host an Origin header would carry.
func originOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

// compile-time guard: the presenter must stay usable as gqlgen's.
var _ graphql.ErrorPresenterFunc = errorPresenter
