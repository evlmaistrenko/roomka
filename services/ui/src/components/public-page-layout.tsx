import type { ReactNode } from "react"
import { useTranslation } from "react-i18next"

import { LanguageSwitcher } from "@/components/language-menu"
import { ThemeToggle } from "@/components/theme-toggle"

// PublicPageLayout frames the pages a visitor reaches without a session: signing
// in, and setting a password with a one-time secret.
export function PublicPageLayout({ children }: { children: ReactNode }) {
	const { i18n } = useTranslation()
	return (
		<div className="relative flex min-h-dvh w-full items-center justify-center p-6 md:p-10">
			<div className="absolute right-4 top-4 flex gap-1">
				{/* Only this browser: preferences need a session to be stored. */}
				<LanguageSwitcher
					onSelect={(language) => void i18n.changeLanguage(language)}
				/>
				<ThemeToggle />
			</div>
			<div className="flex w-full max-w-sm flex-col gap-6">
				<p className="text-center text-lg font-semibold">roomka</p>
				{children}
			</div>
		</div>
	)
}
