import { useEffect, useState } from "react"

import {
	type AnyVariables,
	type Client,
	type CombinedError,
	type DocumentInput,
	createRequest,
} from "@urql/core"
import { useClient } from "urql"
import { onEnd, pipe, subscribe } from "wonka"

export interface LiveSubscriptionState<Data> {
	// The latest emission. The server sends the current state on subscribe, so
	// this is undefined only until the first one arrives.
	data?: Data
	error?: CombinedError
	// How many emissions have arrived, the first one included. A subscription
	// that carries no payload, a bell such as usersChanged, is followed by
	// watching this change.
	emissions: number
}

// What each subscription has delivered so far, per client and request key.
// urql runs one subscription for every component that asks for the same one,
// but hands a component that joins it late only what arrives after it joined,
// and a live view here emits only on change. Without this, a page that mounts
// once its layout has the viewer would wait for a change that may never come.
// The count is kept here too, so every component sees the same number for the
// same emission, and one that joined late still notices the next. A new client,
// as after a sign-in or sign-out, starts empty.
interface Delivered {
	data?: unknown
	emissions: number
	// The result last counted: the shared stream hands the same object to
	// every component on it, and it must be counted once.
	last?: object
}

const delivered = new WeakMap<Client, Map<number, Delivered>>()

function deliveredOf(client: Client, key: number): Delivered {
	let byKey = delivered.get(client)
	if (!byKey) {
		byKey = new Map()
		delivered.set(client, byKey)
	}
	let entry = byKey.get(key)
	if (!entry) {
		entry = { emissions: 0 }
		byKey.set(key, entry)
	}
	return entry
}

// useLiveSubscription keeps a subscription open for as long as the component
// is mounted.
//
// The server ends every subscription on a session, without an error, as soon
// as the access behind it changes, and expects the client to subscribe again:
// the new subscription then carries the new access, or fails with the reason
// it no longer can. urql's useSubscription reports neither ending apart from
// the other, so this follows the result stream itself.
export function useLiveSubscription<
	Data,
	Variables extends AnyVariables = AnyVariables,
>(
	query: DocumentInput<Data, Variables>,
	variables?: Variables,
): LiveSubscriptionState<Data> {
	const client = useClient()
	const request = createRequest(query, variables as Variables)
	const [state, setState] = useState<
		LiveSubscriptionState<Data> & { key?: number }
	>({ emissions: 0 })

	// The request is rebuilt on every render; its key, a hash of the document
	// and the variables, is what says whether it changed.
	const { key } = request
	const [current, setCurrent] = useState(request)
	if (current.key !== key) setCurrent(request)

	useEffect(() => {
		let active = true
		let failed = false
		let unsubscribe = () => {}

		const start = () => {
			;({ unsubscribe } = pipe(
				client.executeSubscription<Data, Variables>(current),
				onEnd(() => {
					// onEnd also fires on our own unsubscribe, and a subscription
					// that failed stays failed until the component remounts.
					if (active && !failed) start()
				}),
				subscribe((result) => {
					failed = result.error !== undefined
					const entry = deliveredOf(client, current.key)
					if (entry.last !== result) {
						entry.last = result
						entry.emissions += 1
						if (result.data !== undefined) entry.data = result.data
					}
					setState({
						key: current.key,
						data: entry.data as Data | undefined,
						error: result.error,
						emissions: entry.emissions,
					})
				}),
			))
		}
		start()

		return () => {
			active = false
			unsubscribe()
		}
	}, [client, current])

	if (state.key === key) {
		return { data: state.data, error: state.error, emissions: state.emissions }
	}
	// Nothing of its own yet: show what the subscription has delivered to
	// whoever was on it first.
	const known = deliveredOf(client, key)
	return { data: known.data as Data | undefined, emissions: known.emissions }
}
