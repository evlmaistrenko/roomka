import { MoreHorizontal, Plus, Search } from "lucide-react"
import { useCallback, useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { CreateUserDialog } from "@/components/create-user-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select"
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@/components/ui/table"
import { UserAccessDialog } from "@/components/user-access-dialog"
import {
	DeleteUserDialog,
	ResetCredentialsDialog,
} from "@/components/user-confirm-dialogs"
import type { UserSort } from "@/graphql/generated/enums"
import type { ManagedUserFragment } from "@/graphql/generated/graphql"
import { USERS_CHANGED_SUBSCRIPTION, USERS_QUERY } from "@/graphql/users"
import { VIEWER_SUBSCRIPTION } from "@/graphql/viewer"
import { useDebouncedValue } from "@/hooks/use-debounced-value"
import { useLiveSubscription } from "@/hooks/use-live-subscription"
import { assignableRank, outranks } from "@/lib/access"
import { formatDateTime } from "@/lib/date-format"
import { describeError } from "@/lib/graphql-errors"
import { useQuery } from "urql"

const PAGE_SIZE = 50

const SEARCH_DELAY_MILLISECONDS = 300

const USER_SORTS = [
	"CREATED_DESC",
	"CREATED_ASC",
	"USERNAME_ASC",
	"USERNAME_DESC",
	"RANK_DESC",
	"RANK_ASC",
] as const satisfies readonly UserSort[]

// What a row can ask the page to open for its user.
interface RowActions {
	onEditAccess: (user: ManagedUserFragment) => void
	onResetCredentials: (user: ManagedUserFragment) => void
	onDelete: (user: ManagedUserFragment) => void
}

export function UsersPage() {
	const { t } = useTranslation()
	const viewer = useLiveSubscription(VIEWER_SUBSCRIPTION).data?.me
	// usersChanged is a bell: every ring, the first one on subscribe included,
	// means "refetch what you are showing".
	const { emissions } = useLiveSubscription(USERS_CHANGED_SUBSCRIPTION)

	const [search, setSearch] = useState("")
	const settledSearch = useDebouncedValue(
		search.trim(),
		SEARCH_DELAY_MILLISECONDS,
	)
	const [sortBy, setSortBy] = useState<UserSort>("CREATED_DESC")

	const [creating, setCreating] = useState(false)
	const [editing, setEditing] = useState<ManagedUserFragment | null>(null)
	const [resetting, setResetting] = useState<ManagedUserFragment | null>(null)
	const [deleting, setDeleting] = useState<ManagedUserFragment | null>(null)

	if (!viewer) return null
	if (!viewer.permissions.includes("USER_MANAGE")) {
		return (
			<p className="py-16 text-center text-sm text-muted-foreground">
				{t("errors.accessDenied")}
			</p>
		)
	}

	const viewerRank = viewer.user.rank
	const maxRank = assignableRank(viewerRank)

	return (
		<div className="flex flex-col gap-6">
			<div className="flex flex-wrap items-end gap-4">
				<div className="flex flex-1 flex-col gap-1">
					<h2 className="text-xl font-semibold">{t("users.title")}</h2>
					<p className="text-sm text-muted-foreground">
						{t("users.description")}
					</p>
				</div>
				{maxRank >= 0 && (
					<Button onClick={() => setCreating(true)}>
						<Plus /> {t("users.create")}
					</Button>
				)}
			</div>

			<div className="flex flex-wrap gap-2">
				<div className="relative min-w-48 flex-1">
					<Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
					<Input
						type="search"
						value={search}
						onChange={(event) => setSearch(event.target.value)}
						placeholder={t("users.search")}
						aria-label={t("users.search")}
						className="pl-8"
					/>
				</div>
				<Select
					value={sortBy}
					onValueChange={(value) => setSortBy(value as UserSort)}
				>
					<SelectTrigger
						className="w-56"
						aria-label={t("users.sortLabel")}
					>
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{USER_SORTS.map((sort) => (
							<SelectItem
								key={sort}
								value={sort}
							>
								{t(`users.sort.${sort}`)}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</div>

			{/* A new filter or sort starts from the first page again: cursors are
			    only valid with the filter and sort that produced them. */}
			<UsersTable
				key={`${settledSearch}\n${sortBy}`}
				search={settledSearch}
				sortBy={sortBy}
				refresh={emissions}
				viewer={{ id: viewer.user.id, rank: viewerRank }}
				actions={{
					onEditAccess: setEditing,
					onResetCredentials: setResetting,
					onDelete: setDeleting,
				}}
			/>

			<CreateUserDialog
				open={creating}
				onOpenChange={setCreating}
				maxRank={maxRank}
			/>
			<UserAccessDialog
				user={editing}
				onOpenChange={(open) => !open && setEditing(null)}
				maxRank={maxRank}
			/>
			<ResetCredentialsDialog
				user={resetting}
				onOpenChange={(open) => !open && setResetting(null)}
			/>
			<DeleteUserDialog
				user={deleting}
				onOpenChange={(open) => !open && setDeleting(null)}
			/>
		</div>
	)
}

interface PageState {
	endCursor?: string | null
	hasNextPage: boolean
	totalCount: number
}

// UsersTable shows the list one page per query, each fetched with the cursor
// the page before it ended on. Every page refetches on its own when the bell
// rings, so the rows already on screen stay where they are.
function UsersTable({
	search,
	sortBy,
	refresh,
	viewer,
	actions,
}: {
	search: string
	sortBy: UserSort
	refresh: number
	viewer: { id: string; rank: number }
	actions: RowActions
}) {
	const { t } = useTranslation()
	const [cursors, setCursors] = useState<(string | null)[]>([null])
	const [pages, setPages] = useState<Record<number, PageState>>({})

	const onPage = useCallback((index: number, page: PageState) => {
		setPages((previous) => {
			const known = previous[index]
			if (
				known &&
				known.endCursor === page.endCursor &&
				known.hasNextPage === page.hasNextPage &&
				known.totalCount === page.totalCount
			) {
				return previous
			}
			return { ...previous, [index]: page }
		})
	}, [])

	const last = pages[cursors.length - 1]
	const totalCount = pages[0]?.totalCount

	return (
		<div className="flex flex-col gap-3">
			<div className="overflow-x-auto rounded-lg border">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>{t("users.columns.user")}</TableHead>
							<TableHead>{t("users.columns.roles")}</TableHead>
							<TableHead className="text-right">
								{t("users.columns.rank")}
							</TableHead>
							<TableHead>{t("users.columns.status")}</TableHead>
							<TableHead>{t("users.columns.created")}</TableHead>
							<TableHead className="w-12">
								<span className="sr-only">{t("users.columns.actions")}</span>
							</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{cursors.map((after, index) => (
							<UsersPageRows
								key={after ?? ""}
								index={index}
								after={after}
								search={search}
								sortBy={sortBy}
								refresh={refresh}
								viewer={viewer}
								actions={actions}
								onPage={onPage}
							/>
						))}
						{totalCount === 0 && (
							<TableRow>
								<TableCell
									colSpan={6}
									className="py-8 text-center text-muted-foreground"
								>
									{t("users.empty")}
								</TableCell>
							</TableRow>
						)}
					</TableBody>
				</Table>
			</div>
			<div className="flex items-center gap-4 text-sm text-muted-foreground">
				{totalCount !== undefined && (
					<span>{t("users.total", { count: totalCount })}</span>
				)}
				<div className="flex-1" />
				{last?.hasNextPage && last.endCursor && (
					<Button
						variant="outline"
						size="sm"
						onClick={() => {
							const next = last.endCursor
							if (next) setCursors((previous) => [...previous, next])
						}}
					>
						{t("users.loadMore")}
					</Button>
				)}
			</div>
		</div>
	)
}

function UsersPageRows({
	index,
	after,
	search,
	sortBy,
	refresh,
	viewer,
	actions,
	onPage,
}: {
	index: number
	after: string | null
	search: string
	sortBy: UserSort
	refresh: number
	viewer: { id: string; rank: number }
	actions: RowActions
	onPage: (index: number, page: PageState) => void
}) {
	const { t } = useTranslation()
	const [{ data, error }, reexecute] = useQuery({
		query: USERS_QUERY,
		variables: {
			filter: search ? { search } : null,
			sortBy,
			after,
			limit: PAGE_SIZE,
		},
	})

	useEffect(() => {
		if (!data) return
		onPage(index, {
			endCursor: data.users.pageInfo.endCursor,
			hasNextPage: data.users.pageInfo.hasNextPage,
			totalCount: data.users.totalCount,
		})
	}, [data, index, onPage])

	// The first ring arrives on subscribe, right after this page's own first
	// fetch: only later ones say something changed.
	const seen = useRef(refresh)
	useEffect(() => {
		if (refresh === seen.current) return
		seen.current = refresh
		reexecute({ requestPolicy: "network-only" })
	}, [refresh, reexecute])

	if (error) {
		return (
			<TableRow>
				<TableCell
					colSpan={6}
					className="py-6 text-center text-destructive"
				>
					{describeError(error)}
				</TableCell>
			</TableRow>
		)
	}

	return (
		<>
			{data?.users.items.map((user) => (
				<TableRow key={user.id}>
					<TableCell>
						<div className="flex flex-col">
							<span className="font-medium">
								{user.displayName}
								{user.id === viewer.id && (
									<span className="ml-2 text-xs font-normal text-muted-foreground">
										{t("users.you")}
									</span>
								)}
							</span>
							<span className="text-xs text-muted-foreground">
								{user.username}
							</span>
						</div>
					</TableCell>
					<TableCell>
						<div className="flex flex-wrap gap-1">
							{user.roles.map((role) => (
								<Badge
									key={role}
									variant="secondary"
								>
									{t(`roles.${role}`)}
								</Badge>
							))}
							{user.directPermissions.map((permission) => (
								<Badge
									key={permission}
									variant="outline"
								>
									{t(`permissions.${permission}`)}
								</Badge>
							))}
						</div>
					</TableCell>
					<TableCell className="text-right tabular-nums">{user.rank}</TableCell>
					<TableCell>
						{user.passwordSetAt ? (
							<span className="text-sm">{t("users.active")}</span>
						) : (
							<Badge variant="outline">{t("users.awaitingPassword")}</Badge>
						)}
					</TableCell>
					<TableCell className="whitespace-nowrap text-sm text-muted-foreground">
						{formatDateTime(user.createdAt)}
					</TableCell>
					<TableCell>
						{outranks(viewer.rank, user.rank) && (
							<UserRowMenu
								user={user}
								actions={actions}
							/>
						)}
					</TableCell>
				</TableRow>
			))}
		</>
	)
}

function UserRowMenu({
	user,
	actions,
}: {
	user: ManagedUserFragment
	actions: RowActions
}) {
	const { t } = useTranslation()
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					variant="ghost"
					size="icon"
					aria-label={t("users.columns.actions")}
				>
					<MoreHorizontal />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuItem onSelect={() => actions.onEditAccess(user)}>
					{t("users.editAccess")}
				</DropdownMenuItem>
				<DropdownMenuItem onSelect={() => actions.onResetCredentials(user)}>
					{t("users.resetCredentials")}
				</DropdownMenuItem>
				<DropdownMenuSeparator />
				<DropdownMenuItem
					variant="destructive"
					onSelect={() => actions.onDelete(user)}
				>
					{t("users.delete")}
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
