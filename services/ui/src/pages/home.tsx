import { useTranslation } from "react-i18next"

import { VIEWER_SUBSCRIPTION } from "@/graphql/viewer"
import { useLiveSubscription } from "@/hooks/use-live-subscription"

export function HomePage() {
	const { t } = useTranslation()
	const { data } = useLiveSubscription(VIEWER_SUBSCRIPTION)
	const user = data?.me.user

	return (
		<div className="mx-auto flex max-w-3xl flex-col gap-6">
			<h1 className="text-2xl font-semibold">
				{user ? t("home.greeting", { name: user.displayName }) : " "}
			</h1>
		</div>
	)
}
