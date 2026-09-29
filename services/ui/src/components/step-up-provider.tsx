import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useMemo,
	useRef,
	useState,
} from "react"
import { useTranslation } from "react-i18next"

import { StepUpContext, type StepUpState } from "@/components/step-up-context"
import { Button } from "@/components/ui/button"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { graphql } from "@/graphql/generated"
import { CHECKS_PASSWORD } from "@/lib/graphql-connection"
import { describeError, hasErrorCode } from "@/lib/graphql-errors"
import { useMutation } from "urql"

const STEP_UP_MUTATION = graphql(`
	mutation StepUp($input: StepUpInput!) {
		stepUp(input: $input) {
			session {
				id
				stepUpExpiresAt
			}
		}
	}
`)

// StepUpProvider owns the one password prompt every step-up goes through.
// Operations that run into STEP_UP_REQUIRED at the same time share it: the
// prompt is asked once and answers them all.
export function StepUpProvider({ children }: { children: ReactNode }) {
	const [open, setOpen] = useState(false)
	const pending = useRef<{
		promise: Promise<boolean>
		resolve: (raised: boolean) => void
	} | null>(null)

	const confirmPassword = useCallback(() => {
		if (!pending.current) {
			let resolve: (raised: boolean) => void = () => {}
			const promise = new Promise<boolean>((settle) => {
				resolve = settle
			})
			pending.current = { promise, resolve }
			setOpen(true)
		}
		return pending.current.promise
	}, [])

	const finish = useCallback((raised: boolean) => {
		pending.current?.resolve(raised)
		pending.current = null
		setOpen(false)
	}, [])

	const state = useMemo<StepUpState>(
		() => ({ confirmPassword }),
		[confirmPassword],
	)

	return (
		<StepUpContext.Provider value={state}>
			{children}
			<Dialog
				open={open}
				onOpenChange={(next) => {
					if (!next) finish(false)
				}}
			>
				{open && <StepUpForm onRaised={() => finish(true)} />}
			</Dialog>
		</StepUpContext.Provider>
	)
}

function StepUpForm({ onRaised }: { onRaised: () => void }) {
	const { t } = useTranslation()
	const [result, stepUp] = useMutation(STEP_UP_MUTATION)

	const submit = async (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault()
		const form = new FormData(event.currentTarget)
		const response = await stepUp(
			{ input: { password: String(form.get("password")) } },
			CHECKS_PASSWORD,
		)
		if (response.data) onRaised()
	}

	const error = result.error
		? hasErrorCode(result.error, "UNAUTHENTICATED")
			? t("stepUp.wrongPassword")
			: describeError(result.error)
		: undefined

	return (
		<DialogContent className="sm:max-w-sm">
			<form
				className="flex flex-col gap-4"
				onSubmit={(event) => void submit(event)}
			>
				<DialogHeader>
					<DialogTitle>{t("stepUp.title")}</DialogTitle>
					<DialogDescription>{t("stepUp.description")}</DialogDescription>
				</DialogHeader>
				<Field data-invalid={error !== undefined}>
					<FieldLabel htmlFor="step-up-password">
						{t("stepUp.password")}
					</FieldLabel>
					<Input
						id="step-up-password"
						name="password"
						type="password"
						autoComplete="current-password"
						aria-invalid={error !== undefined}
						autoFocus
						required
					/>
					<FieldError>{error}</FieldError>
				</Field>
				<DialogFooter>
					<Button
						type="submit"
						disabled={result.fetching}
					>
						{t("stepUp.submit")}
					</Button>
				</DialogFooter>
			</form>
		</DialogContent>
	)
}
