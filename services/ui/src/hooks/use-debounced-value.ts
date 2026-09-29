import { useEffect, useState } from "react"

// useDebouncedValue follows value once it has held still for delay
// milliseconds, so typing in a search box asks the server once, not per key.
export function useDebouncedValue<T>(value: T, delay: number): T {
	const [settled, setSettled] = useState(value)
	useEffect(() => {
		const timer = setTimeout(() => setSettled(value), delay)
		return () => clearTimeout(timer)
	}, [value, delay])
	return settled
}
