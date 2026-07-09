import { useEffect, useState, type ReactNode } from 'react'

import { ThemeProviderContext, type Theme } from '@/components/theme-context'

const STORAGE_KEY = 'roomka-theme'

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(() => {
    // Normalize exactly as the pre-paint script in index.html does, so an
    // unknown/corrupt stored value maps to 'system' in both places and they
    // can't disagree (which would flash the wrong theme on load).
    const stored = localStorage.getItem(STORAGE_KEY)
    return stored === 'light' || stored === 'dark' || stored === 'system'
      ? stored
      : 'system'
  })

  useEffect(() => {
    const root = document.documentElement
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const apply = () => {
      const dark = theme === 'system' ? media.matches : theme === 'dark'
      root.classList.toggle('dark', dark)
    }
    apply()

    if (theme !== 'system') return
    media.addEventListener('change', apply)
    return () => media.removeEventListener('change', apply)
  }, [theme])

  const setTheme = (next: Theme) => {
    localStorage.setItem(STORAGE_KEY, next)
    setThemeState(next)
  }

  return (
    <ThemeProviderContext.Provider value={{ theme, setTheme }}>
      {children}
    </ThemeProviderContext.Provider>
  )
}
