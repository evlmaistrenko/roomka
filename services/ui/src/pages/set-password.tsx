import { type FormEvent, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { Link, useNavigate } from "react-router-dom"

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
import { useAuth } from "@/hooks/use-auth"
import { formatDateTime } from "@/lib/date-format"
import {
	describeError,
	formErrorsOf,
	invalidInputOf,
} from "@/lib/graphql-errors"
import { useMutation, useQuery } from "urql"

const PASSWORD_RESET_INFO_QUERY = graphql(`
	query PasswordResetInfo($secret: String!) {
		passwordResetInfo(secret: $secret) {
			username
			expiresAt
		}
	}
`)

const SET_PASSWORD_MUTATION = graphql(`
	mutation SetPassword($secret: String!, $input: SetPasswordInput!) {
		setPassword(secret: $secret, input: $input) {
			user {
				id
			}
		}
	}
`)

// The secret always arrives in a link, in the URL's fragment
// (/set-password#<secret>): a fragment is never sent to the server or written
// to a proxy's log. It is read once and then taken out of the address bar and
// the history entry.
function secretFromFragment(): string | null {
	let secret = window.location.hash.slice(1)
	try {
		secret = decodeURIComponent(secret)
	} catch {
		// Not percent-encoded after all: take it as it is.
	}
	return secret.trim() || null
}

// SetPasswordPage redeems the one-time link an administrator is given for a new
// or reset account. Setting the password also signs in.
export function SetPasswordPage() {
	const { t } = useTranslation()
	const [secret, setSecret] = useState(secretFromFragment)

	useEffect(() => {
		const clearFragment = () => {
			if (!window.location.hash) return
			window.history.replaceState(
				window.history.state,
				"",
				window.location.pathname + window.location.search,
			)
		}
		clearFragment()
		// A second link pasted into the same tab only changes the fragment,
		// which reloads nothing: the page has to pick the secret up itself.
		const onHashChange = () => {
			const next = secretFromFragment()
			clearFragment()
			if (next) setSecret(next)
		}
		window.addEventListener("hashchange", onHashChange)
		return () => window.removeEventListener("hashchange", onHashChange)
	}, [])

	return (
		<PublicPageLayout>
			{secret ? (
				<PasswordStep
					key={secret}
					secret={secret}
				/>
			) : (
				<LinkProblem message={t("setPassword.noSecret")} />
			)}
		</PublicPageLayout>
	)
}

function PasswordStep({ secret }: { secret: string }) {
	const { t } = useTranslation()
	const navigate = useNavigate()
	const { completeSignIn } = useAuth()
	const [{ data, error, fetching }] = useQuery({
		query: PASSWORD_RESET_INFO_QUERY,
		variables: { secret },
		requestPolicy: "network-only",
	})
	const [result, setPassword] = useMutation(SET_PASSWORD_MUTATION)
	const [mismatch, setMismatch] = useState(false)

	// The secret can lapse between opening the page and submitting it.
	const secretRejected = invalidInputOf(result.error)?.inputPath[0] === "secret"

	if (fetching && !data) return null
	if (error) {
		return <LinkProblem message={describeError(error)} />
	}
	const info = data?.passwordResetInfo
	if (!info || secretRejected) {
		return <LinkProblem message={t("setPassword.invalid")} />
	}

	const submit = async (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault()
		const form = new FormData(event.currentTarget)
		const password = String(form.get("password"))
		if (password !== String(form.get("confirmation"))) {
			setMismatch(true)
			return
		}
		setMismatch(false)
		const response = await setPassword({ secret, input: { password } })
		if (response.data) {
			completeSignIn()
			navigate("/", { replace: true })
		}
	}

	const errors = formErrorsOf(result.error, ["password"])
	const passwordError = errors.fields.password

	return (
		<Card>
			<CardHeader>
				<CardTitle>{t("setPassword.title")}</CardTitle>
				<CardDescription>
					{t("setPassword.description", {
						username: info.username,
						time: formatDateTime(info.expiresAt),
					})}
				</CardDescription>
			</CardHeader>
			<CardContent>
				<form onSubmit={(event) => void submit(event)}>
					{/* Lets a password manager file the new password under the
					    right account. */}
					<input
						type="text"
						name="username"
						autoComplete="username"
						value={info.username}
						readOnly
						hidden
					/>
					<FieldGroup>
						<Field data-invalid={passwordError !== undefined}>
							<FieldLabel htmlFor="password">
								{t("setPassword.password")}
							</FieldLabel>
							<Input
								id="password"
								name="password"
								type="password"
								autoComplete="new-password"
								aria-invalid={passwordError !== undefined}
								autoFocus
								required
							/>
							{passwordError ? (
								<FieldError>{passwordError}</FieldError>
							) : (
								<FieldDescription>
									{t("setPassword.passwordHint")}
								</FieldDescription>
							)}
						</Field>
						<Field data-invalid={mismatch}>
							<FieldLabel htmlFor="confirmation">
								{t("setPassword.confirmation")}
							</FieldLabel>
							<Input
								id="confirmation"
								name="confirmation"
								type="password"
								autoComplete="new-password"
								aria-invalid={mismatch}
								onChange={() => setMismatch(false)}
								required
							/>
							{mismatch && <FieldError>{t("setPassword.mismatch")}</FieldError>}
						</Field>
						<Field>
							<Button
								type="submit"
								disabled={result.fetching}
							>
								{t("setPassword.submit")}
							</Button>
							{errors.form && <FieldError>{errors.form}</FieldError>}
						</Field>
					</FieldGroup>
				</form>
			</CardContent>
		</Card>
	)
}

// LinkProblem stands in for the form when the link cannot be used: it has no
// secret in it, or the secret is no longer valid.
function LinkProblem({ message }: { message: string }) {
	const { t } = useTranslation()
	return (
		<Card>
			<CardHeader>
				<CardTitle>{t("setPassword.title")}</CardTitle>
				<CardDescription className="text-destructive">
					{message}
				</CardDescription>
			</CardHeader>
			<CardContent>
				<Button
					asChild
					variant="outline"
					className="w-full"
				>
					<Link to="/">{t("setPassword.toSignIn")}</Link>
				</Button>
			</CardContent>
		</Card>
	)
}
