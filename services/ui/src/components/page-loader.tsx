import { Loader2 } from "lucide-react"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

// Waits shorter than this show nothing at all: a spinner that flashes for a
// moment reads as a glitch, not as progress.
const SHOW_AFTER_MILLISECONDS = 200

// PageLoader fills the page while something the whole page depends on is on
// its way.
export function PageLoader() {
	const { t } = useTranslation()
	const [shown, setShown] = useState(false)

	useEffect(() => {
		const timer = setTimeout(() => setShown(true), SHOW_AFTER_MILLISECONDS)
		return () => clearTimeout(timer)
	}, [])

	return (
		<div
			className="flex min-h-dvh items-center justify-center"
			role="status"
			aria-label={t("common.loading")}
		>
			{shown && (
				<Loader2 className="size-6 animate-spin text-muted-foreground" />
			)}
		</div>
	)
}
