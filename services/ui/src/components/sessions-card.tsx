import {
	ChevronDown,
	ChevronUp,
	type LucideIcon,
	Monitor,
	Smartphone,
	Tablet,
	Terminal,
} from "lucide-react"
import { type ReactNode, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "@/components/ui/card"
import { graphql } from "@/graphql/generated"
import type { SessionFieldsFragment } from "@/graphql/generated/graphql"
import { useLiveSubscription } from "@/hooks/use-live-subscription"
import { formatDateTime } from "@/lib/date-format"
import { describeError } from "@/lib/graphql-errors"
import { type DeviceKind, describeUserAgent } from "@/lib/user-agent"
import { cn } from "@/lib/utils"
import { useMutation } from "urql"

// How many sessions are listed, live and ended together, most recently used
// first. A user holds at most 10 live ones, so the rest is recent history:
// enough to spot a sign-in that should not be there.
const SESSIONS_SHOWN = 50

// How many of the other sessions show before the rest fold away.
const OTHERS_SHOWN_FOLDED = 5

graphql(`
	fragment SessionFields on Session {
		id
		current
		lastUserAgent
		lastIpAddress
		createdAt
		lastUsedAt
		expiresAt
		endedAt
	}
`)

const SESSIONS_SUBSCRIPTION = graphql(`
	subscription Sessions($limit: Int!) {
		sessions(sortBy: LAST_USED_DESC, limit: $limit) {
			...SessionFields
		}
	}
`)

const REVOKE_SESSION_MUTATION = graphql(`
	mutation RevokeSession($sessionId: ID!) {
		revokeSession(sessionId: $sessionId) {
			id
		}
	}
`)

const REVOKE_OTHER_SESSIONS_MUTATION = graphql(`
	mutation RevokeOtherSessions {
		revokeOtherSessions {
			id
		}
	}
`)

const DEVICE_ICONS: Record<DeviceKind, LucideIcon> = {
	desktop: Monitor,
	mobile: Smartphone,
	tablet: Tablet,
	other: Terminal,
}

// SessionsCard shows where the caller is signed in and where they were: this
// session on its own at the top, then every other one in a single list, the
// ended ones set apart only by how they look. It never refetches: the
// subscription emits the whole list again after every change, so a session
// signed out here turns into an ended one by itself.
export function SessionsCard() {
	const { t } = useTranslation()
	const { data, error: loadError } = useLiveSubscription(
		SESSIONS_SUBSCRIPTION,
		{
			limit: SESSIONS_SHOWN,
		},
	)
	const [revokeResult, revokeSession] = useMutation(REVOKE_SESSION_MUTATION)
	const [revokeOthersResult, revokeOtherSessions] = useMutation(
		REVOKE_OTHER_SESSIONS_MUTATION,
	)

	const sessions = data?.sessions ?? []
	const current = sessions.find((session) => session.current)
	const others = sessions.filter((session) => !session.current)
	const anyOtherLive = others.some((session) => !session.endedAt)
	const [unfolded, setUnfolded] = useState(false)
	const folded = others.length - OTHERS_SHOWN_FOLDED
	const shownOthers =
		unfolded || folded <= 0 ? others : others.slice(0, OTHERS_SHOWN_FOLDED)
	const error = loadError ?? revokeResult.error ?? revokeOthersResult.error

	return (
		<Card>
			<CardHeader>
				<CardTitle>{t("sessions.title")}</CardTitle>
				<CardDescription>{t("sessions.description")}</CardDescription>
			</CardHeader>
			<CardContent className="flex flex-col gap-6">
				{error && (
					<p className="text-sm text-destructive">{describeError(error)}</p>
				)}

				{current && (
					<SessionSection title={t("sessions.current")}>
						<SessionRow session={current} />
					</SessionSection>
				)}

				{data && (
					<SessionSection
						title={t("sessions.others")}
						action={
							anyOtherLive && (
								<Button
									variant="outline"
									size="sm"
									disabled={revokeOthersResult.fetching}
									onClick={() => void revokeOtherSessions({})}
								>
									{t("sessions.signOutOthers")}
								</Button>
							)
						}
					>
						{others.length === 0 ? (
							<p className="py-3 text-sm text-muted-foreground">
								{t("sessions.noOthers")}
							</p>
						) : (
							shownOthers.map((session) => (
								<SessionRow
									key={session.id}
									session={session}
									action={
										!session.endedAt && (
											<Button
												variant="ghost"
												size="sm"
												disabled={revokeResult.fetching}
												onClick={() =>
													void revokeSession({ sessionId: session.id })
												}
											>
												{t("sessions.signOut")}
											</Button>
										)
									}
								/>
							))
						)}
						{folded > 0 && (
							<Button
								variant="ghost"
								size="sm"
								className="my-1 w-full text-muted-foreground"
								aria-expanded={unfolded}
								onClick={() => setUnfolded((open) => !open)}
							>
								{unfolded ? (
									<>
										<ChevronUp /> {t("sessions.showLess")}
									</>
								) : (
									<>
										<ChevronDown /> {t("sessions.showMore", { count: folded })}
									</>
								)}
							</Button>
						)}
					</SessionSection>
				)}
			</CardContent>
		</Card>
	)
}

function SessionSection({
	title,
	action,
	children,
}: {
	title: string
	action?: ReactNode
	children: ReactNode
}) {
	return (
		<section className="flex flex-col gap-1">
			<div className="flex min-h-8 items-center gap-2">
				<h3 className="flex-1 text-sm font-medium">{title}</h3>
				{action}
			</div>
			<div className="flex flex-col divide-y rounded-lg border px-4">
				{children}
			</div>
		</section>
	)
}

function SessionRow({
	session,
	action,
}: {
	session: SessionFieldsFragment
	action?: ReactNode
}) {
	const { t } = useTranslation()
	const device = describeUserAgent(session.lastUserAgent)
	const Icon = DEVICE_ICONS[device.kind]
	const name =
		[device.client, device.system].filter(Boolean).join(" · ") ||
		t("sessions.unknownDevice")
	const ended = session.endedAt != null

	const details = ended
		? [
				t("sessions.endedAt", { time: formatDateTime(session.endedAt ?? "") }),
				session.lastIpAddress,
				t("sessions.signedIn", { time: formatDateTime(session.createdAt) }),
			]
		: [
				session.current
					? t("sessions.signedIn", { time: formatDateTime(session.createdAt) })
					: t("sessions.lastUsed", {
							time: formatDateTime(session.lastUsedAt),
						}),
				session.lastIpAddress,
				t("sessions.expires", { time: formatDateTime(session.expiresAt) }),
			]

	return (
		<div className={cn("flex items-center gap-3 py-3", ended && "opacity-60")}>
			<Icon
				className="size-5 shrink-0 text-muted-foreground"
				aria-hidden
			/>
			<div className="flex min-w-0 flex-1 flex-col gap-0.5">
				<div className="flex flex-wrap items-center gap-x-3 gap-y-0.5">
					<span
						className="truncate text-sm font-medium"
						// The parsed name is a summary; the header itself is one
						// hover away for whoever needs the detail.
						title={session.lastUserAgent ?? undefined}
					>
						{name}
					</span>
					<SessionStatus ended={ended} />
				</div>
				<span className="text-xs text-muted-foreground first-letter:uppercase">
					{details.filter(Boolean).join(" · ")}
				</span>
			</div>
			{action}
		</div>
	)
}

function SessionStatus({ ended }: { ended: boolean }) {
	const { t } = useTranslation()
	return (
		<span
			className={cn(
				"inline-flex items-center gap-1.5 text-xs font-medium",
				ended
					? "text-muted-foreground"
					: "text-emerald-600 dark:text-emerald-400",
			)}
		>
			<span
				className={cn(
					"size-2 rounded-full",
					ended ? "bg-muted-foreground/60" : "bg-emerald-500",
				)}
				aria-hidden
			/>
			{ended ? t("sessions.statusEnded") : t("sessions.statusActive")}
		</span>
	)
}
