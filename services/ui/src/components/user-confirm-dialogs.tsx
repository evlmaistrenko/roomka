import { useState } from "react"
import { useTranslation } from "react-i18next"

import {
	AlertDialog,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { FieldError } from "@/components/ui/field"
import type { ManagedUserFragment } from "@/graphql/generated/graphql"
import {
	DELETE_USER_MUTATION,
	RESET_USER_CREDENTIALS_MUTATION,
} from "@/graphql/users"
import { useStepUpMutation } from "@/hooks/use-step-up"
import { describeError } from "@/lib/graphql-errors"

// Both dialogs stay open until the mutation settles: the confirm button is a
// plain button rather than AlertDialogAction, which would close at once and
// leave a failure nowhere to be shown. Their contents are keyed by the user, so
// a failure shown for one is not carried over to the next.

export function ResetCredentialsDialog({
	user,
	onOpenChange,
}: {
	user: ManagedUserFragment | null
	onOpenChange: (open: boolean) => void
}) {
	return (
		<AlertDialog
			open={user !== null}
			onOpenChange={onOpenChange}
		>
			{user && (
				<ResetCredentialsContent
					key={user.id}
					user={user}
					onClose={() => onOpenChange(false)}
				/>
			)}
		</AlertDialog>
	)
}

function ResetCredentialsContent({
	user,
	onClose,
}: {
	user: ManagedUserFragment
	onClose: () => void
}) {
	const { t } = useTranslation()
	const [result, resetUserCredentials] = useStepUpMutation(
		RESET_USER_CREDENTIALS_MUTATION,
	)
	// Once reset, the dialog tells where the new secret went.
	const [done, setDone] = useState(false)

	const confirm = async () => {
		const response = await resetUserCredentials({ userId: user.id })
		if (response.data) setDone(true)
	}

	return (
		<AlertDialogContent>
			<AlertDialogHeader>
				<AlertDialogTitle>
					{t(done ? "resetCredentials.done" : "resetCredentials.title", {
						name: user.displayName,
					})}
				</AlertDialogTitle>
				<AlertDialogDescription>
					{done ? t("users.linkHint") : t("resetCredentials.description")}
				</AlertDialogDescription>
			</AlertDialogHeader>
			{result.error && !done && (
				<FieldError>{describeError(result.error)}</FieldError>
			)}
			<AlertDialogFooter>
				{done ? (
					<Button onClick={onClose}>{t("common.done")}</Button>
				) : (
					<>
						<AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
						<Button
							variant="destructive"
							disabled={result.fetching}
							onClick={() => void confirm()}
						>
							{t("resetCredentials.confirm")}
						</Button>
					</>
				)}
			</AlertDialogFooter>
		</AlertDialogContent>
	)
}

export function DeleteUserDialog({
	user,
	onOpenChange,
}: {
	user: ManagedUserFragment | null
	onOpenChange: (open: boolean) => void
}) {
	return (
		<AlertDialog
			open={user !== null}
			onOpenChange={onOpenChange}
		>
			{user && (
				<DeleteUserContent
					key={user.id}
					user={user}
					onClose={() => onOpenChange(false)}
				/>
			)}
		</AlertDialog>
	)
}

function DeleteUserContent({
	user,
	onClose,
}: {
	user: ManagedUserFragment
	onClose: () => void
}) {
	const { t } = useTranslation()
	const [result, deleteUser] = useStepUpMutation(DELETE_USER_MUTATION)

	const confirm = async () => {
		const response = await deleteUser({ userId: user.id })
		if (!response.error) onClose()
	}

	return (
		<AlertDialogContent>
			<AlertDialogHeader>
				<AlertDialogTitle>
					{t("deleteUser.title", { name: user.displayName })}
				</AlertDialogTitle>
				<AlertDialogDescription>
					{t("deleteUser.description")}
				</AlertDialogDescription>
			</AlertDialogHeader>
			{result.error && <FieldError>{describeError(result.error)}</FieldError>}
			<AlertDialogFooter>
				<AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
				<Button
					variant="destructive"
					disabled={result.fetching}
					onClick={() => void confirm()}
				>
					{t("deleteUser.confirm")}
				</Button>
			</AlertDialogFooter>
		</AlertDialogContent>
	)
}
