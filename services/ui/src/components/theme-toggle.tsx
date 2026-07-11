import { Monitor, Moon, Sun } from "lucide-react"

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
	const { theme, setTheme } = useTheme()
	const Icon = icons[theme]
	const next = order[(order.indexOf(theme) + 1) % order.length]

	return (
		<Button
			variant="ghost"
			size="icon"
			onClick={() => setTheme(next)}
			title={`Theme: ${theme}`}
			aria-label={`Theme: ${theme}, switch to ${next}`}
		>
			<Icon />
		</Button>
	)
}
