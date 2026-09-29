import { Languages } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { LANGUAGES, LANGUAGE_NAMES, type Language } from "@/lib/language"

// LanguageSwitcher is a menu of its own, for pages outside the account menu.
export function LanguageSwitcher({
	onSelect,
}: {
	onSelect: (language: Language) => void
}) {
	const { t } = useTranslation()
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					variant="ghost"
					size="icon"
					aria-label={t("language.label")}
					title={t("language.label")}
				>
					<Languages />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<LanguageItems onSelect={onSelect} />
			</DropdownMenuContent>
		</DropdownMenu>
	)
}

// LanguageSubmenu is the same choice nested in another menu.
export function LanguageSubmenu({
	onSelect,
}: {
	onSelect: (language: Language) => void
}) {
	const { t } = useTranslation()
	return (
		<DropdownMenuSub>
			<DropdownMenuSubTrigger>
				<Languages /> {t("language.label")}
			</DropdownMenuSubTrigger>
			<DropdownMenuSubContent>
				<LanguageItems onSelect={onSelect} />
			</DropdownMenuSubContent>
		</DropdownMenuSub>
	)
}

function LanguageItems({
	onSelect,
}: {
	onSelect: (language: Language) => void
}) {
	const { i18n } = useTranslation()
	return (
		<DropdownMenuRadioGroup
			value={i18n.language}
			onValueChange={(value) => onSelect(value as Language)}
		>
			{LANGUAGES.map((language) => (
				<DropdownMenuRadioItem
					key={language}
					value={language}
					lang={language}
				>
					{LANGUAGE_NAMES[language]}
				</DropdownMenuRadioItem>
			))}
		</DropdownMenuRadioGroup>
	)
}
