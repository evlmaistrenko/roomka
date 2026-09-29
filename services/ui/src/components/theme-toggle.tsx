import { Monitor, Moon, Sun } from "lucide-react"
import { useTranslation } from "react-i18next"

import type { Theme } from "@/components/theme-context"
import { Button } from "@/components/ui/button"
import { useTheme } from "@/hooks/use-theme"

const order: Theme[] = ["system", "light", "dark"]
const icons: Record<Theme, typeof Monitor> = {
	system: Monitor,
	light: Sun,
	dark: Moon,
}

export function ThemeToggle() {
	const { t } = useTranslation()
	const { theme, setTheme } = useTheme()
	const Icon = icons[theme]
	const next = order[(order.indexOf(theme) + 1) % order.length]

	return (
		<Button
			variant="ghost"
			size="icon"
			onClick={() => setTheme(next)}
			title={t("theme.current", { theme: t(`theme.${theme}`) })}
			aria-label={t("theme.switch", {
				theme: t(`theme.${theme}`),
				next: t(`theme.${next}`),
			})}
		>
			<Icon />
		</Button>
	)
}
