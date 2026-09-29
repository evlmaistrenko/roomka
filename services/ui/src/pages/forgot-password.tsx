import { type FormEvent, useState } from "react"
import { useTranslation } from "react-i18next"
import { Link, useLocation } from "react-router-dom"

import { PublicPageLayout } from "@/components/public-page-layout"
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
import { describeError } from "@/lib/graphql-errors"
import { useMutation } from "urql"

const REQUEST_PASSWORD_RESET_MUTATION = graphql(`
	mutation RequestPasswordReset($username: String!) {
		requestPasswordReset(username: $username)
	}
`)

// ForgotPasswordPage asks for a link to set a new password. The server prints
// the link to its console rather than sending it anywhere, so what the user
// gets here is the instruction to ask whoever runs it. The answer is the same
// whether or not the account exists: saying otherwise would tell a stranger
// which usernames are real.
export function ForgotPasswordPage() {
	const { t } = useTranslation()
	const location = useLocation()
	const typed = (location.state as { username?: string } | null)?.username
	const [result, requestPasswordReset] = useMutation(
		REQUEST_PASSWORD_RESET_MUTATION,
	)
	const [requested, setRequested] = useState(false)

	const submit = async (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault()
		const username = String(new FormData(event.currentTarget).get("username"))
		const response = await requestPasswordReset({ username: username.trim() })
		if (!response.error) setRequested(true)
	}

	return (
		<PublicPageLayout>
			<Card>
				<CardHeader>
					<CardTitle>
						{requested
							? t("forgotPassword.requestedTitle")
							: t("forgotPassword.title")}
					</CardTitle>
					<CardDescription>
						{requested
							? t("forgotPassword.requestedDescription")
							: t("forgotPassword.description")}
					</CardDescription>
				</CardHeader>
				<CardContent>
					{requested ? (
						<Button
							asChild
							variant="outline"
							className="w-full"
						>
							<Link to="/">{t("forgotPassword.toSignIn")}</Link>
						</Button>
					) : (
						<form onSubmit={(event) => void submit(event)}>
							<FieldGroup>
								<Field>
									<FieldLabel htmlFor="username">
										{t("signIn.username")}
									</FieldLabel>
									<Input
										id="username"
										name="username"
										autoComplete="username"
										autoCapitalize="none"
										defaultValue={typed}
										autoFocus
										required
									/>
								</Field>
								<Field>
									<Button
										type="submit"
										disabled={result.fetching}
									>
										{t("forgotPassword.submit")}
									</Button>
									{result.error && (
										<FieldError>{describeError(result.error)}</FieldError>
									)}
									<FieldDescription className="text-center">
										<Link to="/">{t("forgotPassword.toSignIn")}</Link>
									</FieldDescription>
								</Field>
							</FieldGroup>
						</form>
					)}
				</CardContent>
			</Card>
		</PublicPageLayout>
	)
}
