import { type ReactNode, useEffect, useMemo, useRef, useState } from "react"

import { AuthContext, type AuthState } from "@/components/auth-context"
import {
	type GraphQLConnection,
	openGraphQLConnection,
} from "@/lib/graphql-connection"
import { Provider as UrqlProvider } from "urql"

// AuthProvider owns the GraphQL connection and swaps it whenever the session
// cookie changes, so the subscription socket reconnects with the new
// credential (or none) and no data of the previous caller survives in cache.
export function AuthProvider({ children }: { children: ReactNode }) {
	const [authenticationRequired, setAuthenticationRequired] = useState(false)
	const [connection, setConnection] = useState(() =>
		openGraphQLConnection({
			onUnauthenticated: () => setAuthenticationRequired(true),
		}),
	)

	// The replaced connection is disposed after the new one is committed, so
	// nothing still rendered against the old client loses it mid-render. Doing
	// this in an effect cleanup instead would dispose the live connection on
	// StrictMode's simulated unmount.
	const retired = useRef<GraphQLConnection[]>([])
	useEffect(() => {
		for (const previous of retired.current.splice(0)) previous.dispose()
	}, [connection])

	const state = useMemo<AuthState>(() => {
		const reconnect = (required: boolean) => {
			retired.current.push(connection)
			setConnection(
				openGraphQLConnection({
					onUnauthenticated: () => setAuthenticationRequired(true),
				}),
			)
			setAuthenticationRequired(required)
		}
		return {
			authenticationRequired,
			completeSignIn: () => reconnect(false),
			completeSignOut: () => reconnect(true),
		}
	}, [authenticationRequired, connection])

	return (
		<AuthContext.Provider value={state}>
			<UrqlProvider value={connection.client}>{children}</UrqlProvider>
		</AuthContext.Provider>
	)
}
