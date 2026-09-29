import type { LucideIcon } from "lucide-react"
import type { MouseEvent, ReactNode } from "react"
import { NavLink } from "react-router-dom"

import { cn } from "@/lib/utils"

// SideNavLayout frames a page made of parts: its title, a menu of the parts on
// the side, and the part or parts themselves. On a narrow screen the menu
// becomes a strip above the content.
export function SideNavLayout({
	title,
	nav,
	children,
}: {
	title: string
	nav: ReactNode
	children: ReactNode
}) {
	return (
		<div className="mx-auto flex max-w-6xl flex-col gap-6">
			<h1 className="text-2xl font-semibold">{title}</h1>
			<div className="flex flex-col gap-6 md:flex-row md:gap-10">
				<aside className="md:w-52 md:shrink-0">
					<nav className="-mx-1 flex gap-1 overflow-x-auto px-1 [scrollbar-width:none] md:sticky md:top-6 md:flex-col">
						{nav}
					</nav>
				</aside>
				<div className="min-w-0 flex-1">{children}</div>
			</div>
		</div>
	)
}

function itemClass(active: boolean) {
	return cn(
		"flex shrink-0 items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors [&_svg]:size-4 [&_svg]:shrink-0",
		active
			? "bg-muted font-medium text-foreground"
			: "text-muted-foreground hover:bg-muted/50 hover:text-foreground",
	)
}

// SideNavLink is a menu item that is a page of its own.
export function SideNavLink({
	to,
	icon: Icon,
	label,
}: {
	to: string
	icon: LucideIcon
	label: string
}) {
	return (
		<NavLink
			to={to}
			className={({ isActive }) => itemClass(isActive)}
		>
			<Icon /> {label}
		</NavLink>
	)
}

// SideNavAnchor is a menu item that is a part of the same page. It scrolls
// there rather than jumping, and leaves the part's id in the address, so the
// link can be shared.
export function SideNavAnchor({
	id,
	icon: Icon,
	label,
	active,
}: {
	id: string
	icon: LucideIcon
	label: string
	active: boolean
}) {
	const scroll = (event: MouseEvent<HTMLAnchorElement>) => {
		const target = document.getElementById(id)
		if (!target) return
		event.preventDefault()
		target.scrollIntoView({ behavior: "smooth", block: "start" })
		window.history.replaceState(window.history.state, "", `#${id}`)
	}
	return (
		<a
			href={`#${id}`}
			onClick={scroll}
			aria-current={active ? "location" : undefined}
			className={itemClass(active)}
		>
			<Icon /> {label}
		</a>
	)
}
