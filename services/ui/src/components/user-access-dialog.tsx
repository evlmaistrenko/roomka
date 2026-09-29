import { type FormEvent, useState } from "react"
import { useTranslation } from "react-i18next"

import { type Access, AccessFields } from "@/components/access-fields"
import { Button } from "@/components/ui/button"
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import { FieldError, FieldGroup } from "@/components/ui/field"
import type { ManagedUserFragment } from "@/graphql/generated/graphql"
import {
	SET_USER_PERMISSIONS_MUTATION,
	SET_USER_RANK_MUTATION,
	SET_USER_ROLES_MUTATION,
} from "@/graphql/users"
import { useStepUpMutation } from "@/hooks/use-step-up"
import { formErrorsOf } from "@/lib/graphql-errors"
import type { CombinedError } from "@urql/core"

const ACCESS_FIELDS = ["roles", "permissions", "rank"] as const

export function UserAccessDialog({
	user,
	onOpenChange,
	maxRank,
}: {
	// The user being edited; null keeps the dialog closed.
	user: ManagedUserFragment | null
	onOpenChange: (open: boolean) => void
	maxRank: number
}) {
	return (
		<Dialog
			open={user !== null}
			onOpenChange={onOpenChange}
		>
			{user && (
				<UserAccessForm
					key={user.id}
					user={user}
					maxRank={maxRank}
					onSaved={() => onOpenChange(false)}
				/>
			)}
		</Dialog>
	)
}

function UserAccessForm({
	user,
	maxRank,
	onSaved,
}: {
	user: ManagedUserFragment
	maxRank: number
	onSaved: () => void
}) {
	const { t } = useTranslation()
	const [access, setAccess] = useState<Access>({
		roles: user.roles,
		permissions: user.directPermissions,
		rank: user.rank,
	})
	const [saving, setSaving] = useState(false)
	const [error, setError] = useState<CombinedError>()
	const [, setUserRoles] = useStepUpMutation(SET_USER_ROLES_MUTATION)
	const [, setUserPermissions] = useStepUpMutation(
		SET_USER_PERMISSIONS_MUTATION,
	)
	const [, setUserRank] = useStepUpMutation(SET_USER_RANK_MUTATION)

	// Each part goes in its own mutation, so only the parts that changed are
	// sent, one after another; the first failure stops the rest.
	const submit = async (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault()
		setSaving(true)
		setError(undefined)
		const userId = user.id
		const steps = [
			!sameSet(access.roles, user.roles) &&
				(() => setUserRoles({ userId, input: { roles: access.roles } })),
			!sameSet(access.permissions, user.directPermissions) &&
				(() =>
					setUserPermissions({
						userId,
						input: { permissions: access.permissions },
					})),
			access.rank !== user.rank &&
				(() => setUserRank({ userId, input: { rank: access.rank } })),
		]
		for (const step of steps) {
			if (!step) continue
			const response = await step()
			if (response.error) {
				setError(response.error)
				setSaving(false)
				return
			}
		}
		setSaving(false)
		onSaved()
	}

	const errors = formErrorsOf(error, ACCESS_FIELDS)

	return (
		<DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-md">
			<form
				className="flex flex-col gap-6"
				onSubmit={(event) => void submit(event)}
			>
				<DialogHeader>
					<DialogTitle>
						{t("editAccess.title", { name: user.displayName })}
					</DialogTitle>
					<DialogDescription>{t("editAccess.description")}</DialogDescription>
				</DialogHeader>
				<FieldGroup>
					<AccessFields
						value={access}
						onChange={setAccess}
						maxRank={maxRank}
						errors={errors.fields}
					/>
				</FieldGroup>
				{errors.form && <FieldError>{errors.form}</FieldError>}
				<DialogFooter>
					<DialogClose asChild>
						<Button
							type="button"
							variant="outline"
						>
							{t("common.cancel")}
						</Button>
					</DialogClose>
					<Button
						type="submit"
						disabled={saving}
					>
						{t("editAccess.submit")}
					</Button>
				</DialogFooter>
			</form>
		</DialogContent>
	)
}

function sameSet<T>(left: readonly T[], right: readonly T[]): boolean {
	return (
		left.length === right.length && left.every((item) => right.includes(item))
	)
}
