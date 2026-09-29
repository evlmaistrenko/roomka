import { useEffect } from "react"

// How long the page keeps following the part it was opened at while content
// is still arriving.
const FOLLOW_MILLISECONDS = 3000

// useScrollToHash brings the part named in the address (#sessions) into view
// when the page opens. The browser looks for it before anything has rendered,
// and the page is often too short to scroll to it at first render either:
// live data fills in and pushes it down. So the page follows the part as it
// grows, until the reader scrolls on their own or a moment has passed.
export function useScrollToHash(ids: readonly string[]) {
	useEffect(() => {
		const id = window.location.hash.slice(1)
		if (!ids.includes(id)) return

		const scroll = () =>
			document.getElementById(id)?.scrollIntoView({ block: "start" })
		scroll()

		const observer = new ResizeObserver(scroll)
		observer.observe(document.body)
		const stop = () => {
			observer.disconnect()
			clearTimeout(timer)
			for (const event of READER_EVENTS) {
				window.removeEventListener(event, stop)
			}
		}
		const timer = setTimeout(stop, FOLLOW_MILLISECONDS)
		for (const event of READER_EVENTS) {
			window.addEventListener(event, stop, { passive: true })
		}
		return stop
	}, [ids])
}

// What says the reader has taken over the scrolling.
const READER_EVENTS = ["wheel", "touchstart", "keydown", "pointerdown"] as const
