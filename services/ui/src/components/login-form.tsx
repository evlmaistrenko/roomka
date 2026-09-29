import { type FormEvent, useState } from "react"
import { useTranslation } from "react-i18next"
import { Link } from "react-router-dom"

import { Button } from "@/components/ui/button"
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
	Field,
	FieldContent,
	FieldDescription,
	FieldError,
	FieldGroup,
	FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { graphql } from "@/graphql/generated"
import { CHECKS_PASSWORD } from "@/lib/graphql-connection"
import { describeError, hasErrorCode } from "@/lib/graphql-errors"
import { useMutation } from "urql"

const LOGIN_MUTATION = graphql(`
	mutation Login($input: LoginInput!) {
		login(input: $input) {
			user {
				id
			}
		}
	}
`)

// The longest session the schema lets a sign-in ask for, @constraint(max:
// 2592000) on sessionTtlSeconds: 30 days. Without "remember me" the schema's
// default applies, which is a day.
const REMEMBERED_SESSION_SECONDS = 30 * 24 * 60 * 60

export function LoginForm({ onSignedIn }: { onSignedIn: () => void }) {
	const { t } = useTranslation()
	const [result, login] = useMutation(LOGIN_MUTATION)
	// Kept so the forgotten-password page can start from what was typed here.
	const [username, setUsername] = useState("")
	const [remember, setRemember] = useState(true)

	const submit = async (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault()
		const form = new FormData(event.currentTarget)
		const response = await login(
			{
				input: {
					username,
					password: String(form.get("password")),
					...(remember && { sessionTtlSeconds: REMEMBERED_SESSION_SECONDS }),
				},
			},
			CHECKS_PASSWORD,
		)
		if (response.data) onSignedIn()
	}

	const error = result.error
		? hasErrorCode(result.error, "UNAUTHENTICATED")
			? t("signIn.wrongCredentials")
			: describeError(result.error)
		: undefined

	return (
		<Card>
			<CardHeader>
				<CardTitle>{t("signIn.title")}</CardTitle>
				<CardDescription>{t("signIn.description")}</CardDescription>
			</CardHeader>
			<CardContent>
				<form onSubmit={(event) => void submit(event)}>
					<FieldGroup>
						<Field>
							<FieldLabel htmlFor="username">{t("signIn.username")}</FieldLabel>
							<Input
								id="username"
								name="username"
								autoComplete="username"
								autoCapitalize="none"
								value={username}
								onChange={(event) => setUsername(event.target.value)}
								autoFocus
								required
							/>
						</Field>
						<Field data-invalid={error !== undefined}>
							<div className="flex items-center">
								<FieldLabel htmlFor="password">
									{t("signIn.password")}
								</FieldLabel>
								<Link
									to="/forgot-password"
									state={{ username: username.trim() }}
									className="ml-auto text-sm text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
								>
									{t("signIn.forgotPassword")}
								</Link>
							</div>
							<Input
								id="password"
								name="password"
								type="password"
								autoComplete="current-password"
								aria-invalid={error !== undefined}
								required
							/>
							<FieldError>{error}</FieldError>
						</Field>
						<Field orientation="horizontal">
							<Checkbox
								id="remember"
								checked={remember}
								onCheckedChange={(checked) => setRemember(checked === true)}
							/>
							<FieldContent>
								<FieldLabel htmlFor="remember">
									{t("signIn.remember")}
								</FieldLabel>
								<FieldDescription>{t("signIn.rememberHint")}</FieldDescription>
							</FieldContent>
						</Field>
						<Field>
							<Button
								type="submit"
								disabled={result.fetching}
							>
								{t("signIn.submit")}
							</Button>
						</Field>
					</FieldGroup>
				</form>
			</CardContent>
		</Card>
	)
}
