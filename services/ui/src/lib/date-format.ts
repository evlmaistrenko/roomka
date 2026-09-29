import { i18n } from "@/lib/i18n"

const formats = new Map<string, Intl.DateTimeFormat>()

// formatDateTime shows a DateTime from the schema, which is always UTC, in the
// browser's own time zone and the page's current language.
export function formatDateTime(value: string): string {
	let format = formats.get(i18n.language)
	if (!format) {
		format = new Intl.DateTimeFormat(i18n.language, {
			dateStyle: "medium",
			timeStyle: "short",
		})
		formats.set(i18n.language, format)
	}
	return format.format(new Date(value))
}
