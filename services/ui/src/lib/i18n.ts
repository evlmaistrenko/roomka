import { initReactI18next } from "react-i18next"

import { initialLanguage, isLanguage, rememberLanguage } from "@/lib/language"
import { en } from "@/locales/en"
import { ru } from "@/locales/ru"
import i18n from "i18next"

declare module "i18next" {
	interface CustomTypeOptions {
		resources: { translation: typeof en }
	}
}

void i18n.use(initReactI18next).init({
	resources: {
		en: { translation: en },
		ru: { translation: ru },
	},
	lng: initialLanguage(),
	fallbackLng: "en",
	// React escapes what it renders already.
	interpolation: { escapeValue: false },
})

const applyLanguage = (language: string) => {
	document.documentElement.lang = language
	if (isLanguage(language)) rememberLanguage(language)
}
applyLanguage(i18n.language)
i18n.on("languageChanged", applyLanguage)

export { i18n }
