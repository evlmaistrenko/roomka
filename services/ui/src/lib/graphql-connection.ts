import { hasErrorCode } from "@/lib/graphql-errors"
import {
	Client,
	cacheExchange,
	fetchExchange,
	mapExchange,
	subscriptionExchange,
} from "@urql/core"
import { createClient as createSocketClient } from "graphql-ws"

// The close code the schema gives a socket whose session is gone: refused at
// the handshake, or closed when the session expires under it.
const SOCKET_CLOSE_SESSION_INVALID = 4403

const GRAPHQL_PATH = "/graphql"

export interface GraphQLConnection {
	client: Client
	dispose: () => void
}

// openGraphQLConnection builds the client for one credential: HTTP for queries
// and mutations, one WebSocket for subscriptions. The session cookie rides on
// the socket's handshake and nothing re-authenticates inside it, so a socket
// is bound to whichever cookie the browser held when it opened. A sign-in or
// sign-out therefore opens a new connection rather than reusing this one,
// which also drops the cache of the previous caller's data.
export function openGraphQLConnection({
	onUnauthenticated,
}: {
	onUnauthenticated: () => void
}): GraphQLConnection {
	const socketURL = new URL(GRAPHQL_PATH, window.location.href)
	socketURL.protocol = socketURL.protocol === "https:" ? "wss:" : "ws:"

	const socket = createSocketClient({
		url: socketURL.href,
		// Open the socket with the first subscription, not the page: a signed
		// out visitor may never need one.
		lazy: true,
		// graphql-ws retries 4403 by default, but a dead session does not come
		// back: the retry would only be refused again.
		shouldRetry: (event) => !isSessionInvalidClose(event),
		on: {
			closed: (event) => {
				if (isSessionInvalidClose(event)) onUnauthenticated()
			},
		},
	})

	const client = new Client({
		url: GRAPHQL_PATH,
		// Only this origin receives the SameSite=Strict session cookie, and the
		// server accepts it only from here.
		fetchOptions: { credentials: "same-origin" },
		exchanges: [
			mapExchange({
				onError(error, operation) {
					// login and stepUp answer a wrong password with UNAUTHENTICATED
					// too. There it is the form's to report, not a sign that the
					// session is gone.
					if (operation.context.checksPassword) return
					if (hasErrorCode(error, "UNAUTHENTICATED")) onUnauthenticated()
				},
			}),
			cacheExchange,
			fetchExchange,
			subscriptionExchange({
				forwardSubscription(request) {
					const input = { ...request, query: request.query ?? "" }
					return {
						subscribe(sink) {
							const unsubscribe = socket.subscribe(input, sink)
							return { unsubscribe }
						},
					}
				},
			}),
		],
	})

	return {
		client,
		// dispose waits for a connection in progress to close it, and rejects
		// when that connection fails instead. A socket that never opened has
		// nothing left to close.
		dispose: () => {
			Promise.resolve(socket.dispose()).catch(() => {})
		},
	}
}

function isSessionInvalidClose(event: unknown): boolean {
	return (
		event instanceof CloseEvent && event.code === SOCKET_CLOSE_SESSION_INVALID
	)
}

// CHECKS_PASSWORD marks an operation whose UNAUTHENTICATED means a wrong
// password rather than a missing session: pass it as the context of login and
// stepUp.
export const CHECKS_PASSWORD = { checksPassword: true } as const
