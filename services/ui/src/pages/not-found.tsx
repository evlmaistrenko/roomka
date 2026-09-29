import { useTranslation } from "react-i18next"
import { Link } from "react-router-dom"

import { Button } from "@/components/ui/button"

export function NotFoundPage() {
	const { t } = useTranslation()
	return (
		<div className="flex flex-col items-center gap-4 py-16 text-center">
			<h1 className="text-2xl font-semibold">{t("notFound.title")}</h1>
			<Button
				asChild
				variant="outline"
			>
				<Link to="/">{t("notFound.home")}</Link>
			</Button>
		</div>
	)
}
