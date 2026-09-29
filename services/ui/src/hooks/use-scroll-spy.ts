import { useEffect, useState } from "react"

// useScrollSpy tells which of the sections, given by element id in page order,
// is being read: the last one whose top has scrolled past the line offset
// pixels below the top of the viewport. At the very bottom of the page it is
// the last section, which may be too short to ever reach the line.
export function useScrollSpy(
	ids: readonly string[],
	offset: number,
): string | undefined {
	const [active, setActive] = useState<string | undefined>(ids[0])

	useEffect(() => {
		let frame = 0
		const update = () => {
			cancelAnimationFrame(frame)
			frame = requestAnimationFrame(() => {
				let current: string | undefined = ids[0]
				for (const id of ids) {
					const top = document.getElementById(id)?.getBoundingClientRect().top
					if (top !== undefined && top <= offset) current = id
				}
				const bottom =
					window.innerHeight + window.scrollY >=
					document.documentElement.scrollHeight - 2
				if (bottom && window.scrollY > 0) current = ids.at(-1)
				setActive(current)
			})
		}
		update()
		window.addEventListener("scroll", update, { passive: true })
		window.addEventListener("resize", update)
		return () => {
			cancelAnimationFrame(frame)
			window.removeEventListener("scroll", update)
			window.removeEventListener("resize", update)
		}
	}, [ids, offset])

	return active
}
