export const LANGUAGES = ["en", "ru"] as const

export type Language = (typeof LANGUAGES)[number]

// Each language is named in itself, so a reader who can't read the current
// one still finds their own.
export const LANGUAGE_NAMES: Record<Language, string> = {
	en: "English",
	ru: "Русский",
}

// The key under which the signed-in user's choice lives in their server-side
// preferences, which follow them from browser to browser.
export const LANGUAGE_PREFERENCE_KEY = "language"

// The last language this browser showed. It is what a signed-out visitor sees,
// since preferences can only be read once signed in.
const STORAGE_KEY = "roomka-language"

export function isLanguage(value: unknown): value is Language {
	return LANGUAGES.includes(value as Language)
}

// initialLanguage picks what to show before any preference is known: the last
// language used here, else the first supported one the browser asks for.
export function initialLanguage(): Language {
	try {
		const stored = localStorage.getItem(STORAGE_KEY)
		if (isLanguage(stored)) return stored
	} catch {
		// Storage unavailable: fall through to the browser's languages.
	}
	for (const tag of navigator.languages) {
		const primary = tag.split("-")[0]?.toLowerCase()
		if (isLanguage(primary)) return primary
	}
	return "en"
}

export function rememberLanguage(language: Language) {
	try {
		localStorage.setItem(STORAGE_KEY, language)
	} catch {
		// Nothing to do: the choice just won't outlive the tab.
	}
}
