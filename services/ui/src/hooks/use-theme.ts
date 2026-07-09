import { useContext } from "react"

import { ThemeProviderContext } from "@/components/theme-context"

export function useTheme() {
	return useContext(ThemeProviderContext)
}
