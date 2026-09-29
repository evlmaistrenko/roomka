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
import {
	Field,
	FieldDescription,
	FieldError,
	FieldGroup,
	FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { CREATE_USER_MUTATION } from "@/graphql/users"
import { useStepUpMutation } from "@/hooks/use-step-up"
import { formErrorsOf } from "@/lib/graphql-errors"

const CREATE_USER_FIELDS = [
	"displayName",
	"username",
	"roles",
	"permissions",
	"rank",
] as const

export function CreateUserDialog({
	open,
	onOpenChange,
	maxRank,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	maxRank: number
}) {
	return (
		<Dialog
			open={open}
			onOpenChange={onOpenChange}
		>
			{/* Mounted only while open, so every opening starts blank. */}
			{open && <CreateUserForm maxRank={maxRank} />}
		</Dialog>
	)
}

function CreateUserForm({ maxRank }: { maxRank: number }) {
	const { t } = useTranslation()
	const [result, createUser] = useStepUpMutation(CREATE_USER_MUTATION)
	const [access, setAccess] = useState<Access>({
		roles: [],
		permissions: [],
		rank: 0,
	})
	const [created, setCreated] = useState<string>()

	const submit = async (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault()
		const form = new FormData(event.currentTarget)
		const response = await createUser({
			input: {
				displayName: String(form.get("displayName")),
				username: String(form.get("username")),
				roles: access.roles,
				permissions: access.permissions,
				rank: access.rank,
			},
		})
		if (response.data) setCreated(response.data.createUser.displayName)
	}

	if (created !== undefined) {
		return (
			<DialogContent className="sm:max-w-md">
				<DialogHeader>
					<DialogTitle>
						{t("createUser.created", { name: created })}
					</DialogTitle>
					<DialogDescription>{t("users.linkHint")}</DialogDescription>
				</DialogHeader>
				<DialogFooter>
					<DialogClose asChild>
						<Button>{t("common.done")}</Button>
					</DialogClose>
				</DialogFooter>
			</DialogContent>
		)
	}

	const errors = formErrorsOf(result.error, CREATE_USER_FIELDS)

	return (
		<DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-md">
			<form
				className="flex flex-col gap-6"
				onSubmit={(event) => void submit(event)}
			>
				<DialogHeader>
					<DialogTitle>{t("createUser.title")}</DialogTitle>
					<DialogDescription>{t("createUser.description")}</DialogDescription>
				</DialogHeader>
				<FieldGroup>
					<Field data-invalid={errors.fields.displayName !== undefined}>
						<FieldLabel htmlFor="displayName">
							{t("userForm.displayName")}
						</FieldLabel>
						<Input
							id="displayName"
							name="displayName"
							autoComplete="off"
							aria-invalid={errors.fields.displayName !== undefined}
							autoFocus
							required
						/>
						{errors.fields.displayName ? (
							<FieldError>{errors.fields.displayName}</FieldError>
						) : (
							<FieldDescription>
								{t("userForm.displayNameHint")}
							</FieldDescription>
						)}
					</Field>
					<Field data-invalid={errors.fields.username !== undefined}>
						<FieldLabel htmlFor="username">{t("userForm.username")}</FieldLabel>
						<Input
							id="username"
							name="username"
							autoComplete="off"
							autoCapitalize="none"
							aria-invalid={errors.fields.username !== undefined}
							required
						/>
						{errors.fields.username ? (
							<FieldError>{errors.fields.username}</FieldError>
						) : (
							<FieldDescription>{t("userForm.usernameHint")}</FieldDescription>
						)}
					</Field>
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
						disabled={result.fetching}
					>
						{t("createUser.submit")}
					</Button>
				</DialogFooter>
			</form>
		</DialogContent>
	)
}
