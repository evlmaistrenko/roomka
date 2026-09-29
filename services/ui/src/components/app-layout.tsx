import { Suspense, use } from "react"
import { useTranslation } from "react-i18next"
import { Link, NavLink, Outlet } from "react-router-dom"

import { PageLoader } from "@/components/page-loader"
import { StepUpProvider } from "@/components/step-up-provider"
import { ThemeToggle } from "@/components/theme-toggle"
import { UserMenu } from "@/components/user-menu"
import { VIEWER_SUBSCRIPTION } from "@/graphql/viewer"
import { useLanguagePreference } from "@/hooks/use-language-preference"
import { useLiveSubscription } from "@/hooks/use-live-subscription"
import { cn } from "@/lib/utils"

// AppLayout holds what has to outlive the wait below: the viewer subscription,
// which a suspended component would drop and open again.
export function AppLayout() {
	const languageSettled = useLanguagePreference()
	return (
		<Suspense fallback={<PageLoader />}>
			<AppShell languageSettled={languageSettled} />
		</Suspense>
	)
}

// AppShell is the page itself. It shows only once the signed-in user's
// language is known, so nothing on it is ever seen in another one first.
function AppShell({ languageSettled }: { languageSettled: Promise<void> }) {
	use(languageSettled)
	const { t } = useTranslation()
	const permissions =
		useLiveSubscription(VIEWER_SUBSCRIPTION).data?.me.permissions

	// A link is shown to whoever may use the page behind it; the page itself
	// still gets the server's answer.
	const links = [
		{ to: "/", label: t("nav.home"), shown: true, end: true },
		{
			// Lit on every page under it, not only on its first one.
			to: "/admin",
			label: t("nav.admin"),
			shown: permissions?.includes("USER_MANAGE") ?? false,
			end: false,
		},
	].filter((link) => link.shown)

	return (
		<StepUpProvider>
			<div className="flex min-h-dvh flex-col">
				<header className="flex h-14 items-center gap-4 border-b px-4 md:gap-6">
					<Link
						to="/"
						className="font-semibold"
					>
						roomka
					</Link>
					{/* On a narrow screen the links scroll sideways; the controls on the
					    right always stay in view. */}
					<nav className="flex min-w-0 flex-1 items-center gap-4 overflow-x-auto whitespace-nowrap text-sm [scrollbar-width:none]">
						{links.map((link) => (
							<NavLink
								key={link.to}
								to={link.to}
								end={link.end}
								className={({ isActive }) =>
									cn(
										"transition-colors hover:text-foreground",
										isActive ? "text-foreground" : "text-muted-foreground",
									)
								}
							>
								{link.label}
							</NavLink>
						))}
					</nav>
					<div className="flex shrink-0 items-center gap-2">
						<ThemeToggle />
						<UserMenu />
					</div>
				</header>
				<main className="flex-1 p-4 md:p-6">
					<Outlet />
				</main>
			</div>
		</StepUpProvider>
	)
}
