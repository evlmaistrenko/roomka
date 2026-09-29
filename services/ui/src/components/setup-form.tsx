import type { FormEvent } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "@/components/ui/card"
import {
	Field,
	FieldDescription,
	FieldError,
	FieldGroup,
	FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { graphql } from "@/graphql/generated"
import { formErrorsOf } from "@/lib/graphql-errors"
import { useMutation } from "urql"

const SETUP_MUTATION = graphql(`
	mutation Setup($input: SetupInput!) {
		setup(input: $input) {
			user {
				id
			}
		}
	}
`)

const SETUP_FIELDS = ["displayName", "username", "password"] as const

type SetupField = (typeof SETUP_FIELDS)[number]

// SetupForm creates the root account on a server that has no users yet.
export function SetupForm({
	onSignedIn,
	onAlreadySetUp,
}: {
	onSignedIn: () => void
	// Someone else finished setup first: setup answers null once a user exists.
	onAlreadySetUp: () => void
}) {
	const { t } = useTranslation()
	const [result, setup] = useMutation(SETUP_MUTATION)

	const submit = async (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault()
		const form = new FormData(event.currentTarget)
		const response = await setup({
			input: {
				displayName: String(form.get("displayName")),
				username: String(form.get("username")),
				password: String(form.get("password")),
			},
		})
		if (response.error) return
		if (response.data?.setup) onSignedIn()
		else onAlreadySetUp()
	}

	const errors = formErrorsOf(result.error, SETUP_FIELDS)
	const fieldError = (field: SetupField) => errors.fields[field]
	const formError = errors.form

	const field = (
		name: SetupField,
		input: React.ComponentProps<typeof Input>,
	) => {
		const error = fieldError(name)
		return (
			<Field data-invalid={error !== undefined}>
				<FieldLabel htmlFor={name}>{t(`setup.${name}`)}</FieldLabel>
				<Input
					id={name}
					name={name}
					aria-invalid={error !== undefined}
					required
					{...input}
				/>
				{error ? (
					<FieldError>{error}</FieldError>
				) : (
					<FieldDescription>{t(`setup.${name}Hint`)}</FieldDescription>
				)}
			</Field>
		)
	}

	return (
		<Card>
			<CardHeader>
				<CardTitle>{t("setup.title")}</CardTitle>
				<CardDescription>{t("setup.description")}</CardDescription>
			</CardHeader>
			<CardContent>
				<form onSubmit={(event) => void submit(event)}>
					<FieldGroup>
						{field("displayName", { autoComplete: "name", autoFocus: true })}
						{field("username", {
							autoComplete: "username",
							autoCapitalize: "none",
						})}
						{field("password", {
							type: "password",
							autoComplete: "new-password",
						})}
						<Field>
							<Button
								type="submit"
								disabled={result.fetching}
							>
								{t("setup.submit")}
							</Button>
							{formError && <FieldError>{formError}</FieldError>}
						</Field>
					</FieldGroup>
				</form>
			</CardContent>
		</Card>
	)
}
