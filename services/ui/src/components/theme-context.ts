import { createContext } from "react"

export type Theme = "light" | "dark" | "system"

export type ThemeProviderState = {
	theme: Theme
	setTheme: (theme: Theme) => void
}

export const ThemeProviderContext = createContext<ThemeProviderState>({
	theme: "system",
	setTheme: () => undefined,
})
