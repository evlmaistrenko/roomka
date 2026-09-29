import { Users } from "lucide-react"
import { useTranslation } from "react-i18next"
import { Outlet } from "react-router-dom"

import { SideNavLayout, SideNavLink } from "@/components/side-nav-layout"
import { VIEWER_SUBSCRIPTION } from "@/graphql/viewer"
import { useLiveSubscription } from "@/hooks/use-live-subscription"

// AdminLayout holds the pages for running the server rather than using it. The
// menu lists the ones the caller's permissions reach; each page still gets the
// server's own answer.
export function AdminLayout() {
	const { t } = useTranslation()
	const permissions =
		useLiveSubscription(VIEWER_SUBSCRIPTION).data?.me.permissions
	if (!permissions) return null

	const canManageUsers = permissions.includes("USER_MANAGE")
	if (!canManageUsers) {
		return (
			<p className="py-16 text-center text-sm text-muted-foreground">
				{t("errors.accessDenied")}
			</p>
		)
	}

	return (
		<SideNavLayout
			title={t("admin.title")}
			nav={
				<SideNavLink
					to="/admin/users"
					icon={Users}
					label={t("users.title")}
				/>
			}
		>
			<Outlet />
		</SideNavLayout>
	)
}
