import { Monitor, MonitorSmartphone, Moon, Palette, Sun } from "lucide-react"
import { useTranslation } from "react-i18next"

import { SessionsCard } from "@/components/sessions-card"
import { SideNavAnchor, SideNavLayout } from "@/components/side-nav-layout"
import type { Theme } from "@/components/theme-context"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
	Field,
	FieldContent,
	FieldDescription,
	FieldGroup,
	FieldLabel,
} from "@/components/ui/field"
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select"
import { useChooseLanguage } from "@/hooks/use-language-preference"
import { useScrollSpy } from "@/hooks/use-scroll-spy"
import { useScrollToHash } from "@/hooks/use-scroll-to-hash"
import { useTheme } from "@/hooks/use-theme"
import { LANGUAGES, LANGUAGE_NAMES, type Language } from "@/lib/language"

const THEMES = [
	{ value: "system", icon: Monitor },
	{ value: "light", icon: Sun },
	{ value: "dark", icon: Moon },
] as const satisfies readonly { value: Theme; icon: typeof Monitor }[]

// The page's parts, in page order: each is an element id the side menu links
// to and follows as the page scrolls.
const SECTIONS = ["appearance", "sessions"] as const

// A part counts as the one being read once its top is this far from the top
// of the viewport or closer: about where the eye lands below the header.
const READING_LINE_PIXELS = 120

export function SettingsPage() {
	const { t } = useTranslation()
	const active = useScrollSpy(SECTIONS, READING_LINE_PIXELS)

	useScrollToHash(SECTIONS)

	return (
		<SideNavLayout
			title={t("settings.title")}
			nav={
				<>
					<SideNavAnchor
						id="appearance"
						icon={Palette}
						label={t("settings.appearance")}
						active={active === "appearance"}
					/>
					<SideNavAnchor
						id="sessions"
						icon={MonitorSmartphone}
						label={t("sessions.title")}
						active={active === "sessions"}
					/>
				</>
			}
		>
			<div className="flex max-w-3xl flex-col gap-6">
				<section
					id="appearance"
					className="scroll-mt-6"
				>
					<AppearanceCard />
				</section>
				<section
					id="sessions"
					className="scroll-mt-6"
				>
					<SessionsCard />
				</section>
			</div>
		</SideNavLayout>
	)
}

// AppearanceCard repeats the switches in the header and the account menu, for
// whoever looks for them here.
function AppearanceCard() {
	const { t, i18n } = useTranslation()
	const chooseLanguage = useChooseLanguage()
	const { theme, setTheme } = useTheme()

	return (
		<Card>
			<CardHeader>
				<CardTitle>{t("settings.appearance")}</CardTitle>
			</CardHeader>
			<CardContent>
				<FieldGroup>
					<Field orientation="responsive">
						<FieldContent>
							<FieldLabel htmlFor="language">
								{t("settings.language")}
							</FieldLabel>
							<FieldDescription>{t("settings.languageHint")}</FieldDescription>
						</FieldContent>
						<Select
							value={i18n.language}
							onValueChange={(value) => chooseLanguage(value as Language)}
						>
							<SelectTrigger
								id="language"
								className="sm:w-48"
							>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{LANGUAGES.map((language) => (
									<SelectItem
										key={language}
										value={language}
										lang={language}
									>
										{LANGUAGE_NAMES[language]}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</Field>
					<Field orientation="responsive">
						<FieldContent>
							<FieldLabel htmlFor="theme">{t("settings.theme")}</FieldLabel>
							<FieldDescription>{t("settings.themeHint")}</FieldDescription>
						</FieldContent>
						<Select
							value={theme}
							onValueChange={(value) => setTheme(value as Theme)}
						>
							<SelectTrigger
								id="theme"
								className="sm:w-48"
							>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{THEMES.map(({ value, icon: Icon }) => (
									<SelectItem
										key={value}
										value={value}
									>
										<Icon /> {t(`settings.themes.${value}`)}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</Field>
				</FieldGroup>
			</CardContent>
		</Card>
	)
}
