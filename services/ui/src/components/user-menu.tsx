import { LogOut, Settings } from "lucide-react"
import { useTranslation } from "react-i18next"
import { Link } from "react-router-dom"

import { LanguageSubmenu } from "@/components/language-menu"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { graphql } from "@/graphql/generated"
import { VIEWER_SUBSCRIPTION } from "@/graphql/viewer"
import { useAuth } from "@/hooks/use-auth"
import { useChooseLanguage } from "@/hooks/use-language-preference"
import { useLiveSubscription } from "@/hooks/use-live-subscription"
import { useMutation } from "urql"

const LOGOUT_MUTATION = graphql(`
	mutation Logout {
		logout
	}
`)

export function UserMenu() {
	const { t } = useTranslation()
	const chooseLanguage = useChooseLanguage()
	const { completeSignOut } = useAuth()
	const { data } = useLiveSubscription(VIEWER_SUBSCRIPTION)
	const [logoutResult, logout] = useMutation(LOGOUT_MUTATION)

	const signOut = async () => {
		const response = await logout({})
		// UNAUTHENTICATED means the session was already gone: signed out either
		// way.
		if (!response.error?.networkError) completeSignOut()
	}

	const user = data?.me.user
	if (!user) return null

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					variant="ghost"
					size="icon"
					className="rounded-full"
					aria-label={t("account.menu")}
				>
					<Avatar className="size-8">
						<AvatarFallback>{initialsOf(user.displayName)}</AvatarFallback>
					</Avatar>
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				align="end"
				className="min-w-56"
			>
				<DropdownMenuLabel className="flex flex-col">
					<span className="truncate">{user.displayName}</span>
					<span className="truncate text-xs font-normal text-muted-foreground">
						{user.username}
					</span>
				</DropdownMenuLabel>
				<DropdownMenuSeparator />
				<DropdownMenuItem asChild>
					<Link to="/settings">
						<Settings /> {t("account.settings")}
					</Link>
				</DropdownMenuItem>
				<LanguageSubmenu onSelect={chooseLanguage} />
				<DropdownMenuItem
					disabled={logoutResult.fetching}
					onSelect={() => void signOut()}
				>
					<LogOut /> {t("account.signOut")}
				</DropdownMenuItem>
				<DropdownMenuSeparator />
				<DropdownMenuLabel className="text-xs font-normal text-muted-foreground">
					roomka {__APP_VERSION__}
				</DropdownMenuLabel>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}

function initialsOf(name: string): string {
	return name
		.split(/\s+/)
		.filter(Boolean)
		.slice(0, 2)
		.map((word) => word[0]?.toUpperCase())
		.join("")
}
