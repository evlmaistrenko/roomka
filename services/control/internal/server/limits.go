package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"control/internal/network"
	"control/internal/ratelimit"
	"control/internal/schema"
)

// The limits of the HTTP layer itself. The proxy in front is expected to set
// its own a little tighter; these hold when something reaches the server
// directly.
const (
	// readHeaderTimeout bounds how long a client may take to send its headers,
	// which is what stops a slow client from holding connections open for free.
	readHeaderTimeout = 10 * time.Second
	// requestTimeout bounds reading the rest of a request and writing its
	// response. It is set per request rather than on the server because the
	// server-wide timeouts would also cut every subscription socket at the same
	// age, and a socket is meant to stay open.
	requestTimeout = 30 * time.Second
	// idleTimeout closes a kept-alive connection nobody is using.
	idleTimeout     = 2 * time.Minute
	maxHeaderBytes  = 64 << 10
	maxRequestBytes = 1 << 20
	// maxSocketMessageBytes is the largest frame a subscription socket accepts.
	// A frame is one GraphQL operation, which is nowhere near this.
	maxSocketMessageBytes = 64 << 10
	// socketInitTimeout is how long a socket may stay open without introducing
	// itself with connection_init.
	socketInitTimeout = 10 * time.Second
	// socketPingInterval is how often the server pings a socket. It sits well
	// under the shortest idle timeout a middlebox is likely to impose (15
	// seconds has been seen on a local antivirus), and a socket that misses two
	// pongs is closed.
	socketPingInterval = 10 * time.Second
)

// HTTPServer wraps a handler in a server with the timeouts above. main listens
// with it; tests use their own.
func HTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
}

// isUpgrade reports whether a request asks to become a WebSocket.
func isUpgrade(request *http.Request) bool {
	return request.Header.Get("Upgrade") != ""
}

// withRequestLimits applies what every request is subject to: the per-address
// rate, and — for anything that is not becoming a socket — a deadline and a
// size limit on the body.
func withRequestLimits(policy ratelimit.Policy, next http.Handler) http.Handler {
	requests := ratelimit.NewKeyed(policy.Requests)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if allowed, wait := requests.Allow(network.ClientAddress(request)); !allowed {
			refuse(writer, wait)
			return
		}
		if !isUpgrade(request) {
			// A body declared too big is refused before any of it is read. One
			// that declares no length is cut off at the limit instead; gqlgen then
			// reports the failed read as a GraphQL error rather than a status.
			if request.ContentLength > maxRequestBytes {
				http.Error(writer, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			controller := http.NewResponseController(writer)
			deadline := time.Now().Add(requestTimeout)
			_ = controller.SetReadDeadline(deadline)
			_ = controller.SetWriteDeadline(deadline)
			request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBytes)
		}
		next.ServeHTTP(writer, request)
	})
}

// withSocketLimit caps the subscription sockets one address holds open. The
// gqlgen handler returns only when its socket closes, so the slot is held for
// exactly the socket's life.
func withSocketLimit(policy ratelimit.Policy, next http.Handler) http.Handler {
	sockets := ratelimit.NewConcurrent(policy.SocketsPerAddress)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !isUpgrade(request) {
			next.ServeHTTP(writer, request)
			return
		}
		address := network.ClientAddress(request)
		if !sockets.Acquire(address) {
			// Nothing to wait for: a slot frees when a socket closes, not on a
			// clock.
			http.Error(writer, "too many open sockets", http.StatusTooManyRequests)
			return
		}
		defer sockets.Release(address)
		next.ServeHTTP(writer, request)
	})
}

// refuse answers a request that came too soon.
func refuse(writer http.ResponseWriter, wait time.Duration) {
	writer.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(wait)))
	http.Error(writer, "too many requests", http.StatusTooManyRequests)
}

// socketState is what one subscription socket is allowed: how fast it may start
// operations, and how many subscriptions it may hold at once.
type socketState struct {
	operations    *ratelimit.Single
	subscriptions chan struct{}
}

type socketStateKey struct{}

// withSocketState gives a socket its own limits. It is called from the socket's
// init, whose context every operation on the socket inherits.
func withSocketState(ctx context.Context, policy ratelimit.Policy) context.Context {
	return context.WithValue(ctx, socketStateKey{}, &socketState{
		operations:    ratelimit.NewSingle(policy.SocketOperations),
		subscriptions: make(chan struct{}, policy.SubscriptionsPerSocket),
	})
}

// socketLimits enforces socketState on every operation started over a socket.
// Operations over plain HTTP carry no state and pass straight through: they are
// already counted one request at a time.
type socketLimits struct{}

var (
	_ graphql.HandlerExtension     = socketLimits{}
	_ graphql.OperationInterceptor = socketLimits{}
)

func (socketLimits) ExtensionName() string                   { return "SocketLimits" }
func (socketLimits) Validate(graphql.ExecutableSchema) error { return nil }

func (socketLimits) InterceptOperation(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
	state, ok := ctx.Value(socketStateKey{}).(*socketState)
	if !ok {
		return next(ctx)
	}
	if allowed, wait := state.operations.Allow(); !allowed {
		return refuseOperation(ctx, schema.RateLimited(wait))
	}
	if graphql.GetOperationContext(ctx).Operation.Operation != ast.Subscription {
		return next(ctx)
	}
	select {
	case state.subscriptions <- struct{}{}:
	default:
		return refuseOperation(ctx, schema.Coded(schema.ErrorCodeLimitReached,
			"too many subscriptions on this socket"))
	}
	// The operation's context ends when the subscription does, however it
	// ends: completed, stopped by the client, or taken down with the socket.
	go func() {
		<-ctx.Done()
		<-state.subscriptions
	}()
	return next(ctx)
}

// refuseOperation answers an operation with one error and nothing else. gqlgen
// requires a one-shot handler here, or a subscription would keep asking it for
// the next response forever.
func refuseOperation(ctx context.Context, err error) graphql.ResponseHandler {
	return graphql.OneShot(&graphql.Response{Errors: gqlerror.List{errorPresenter(ctx, err)}})
}
