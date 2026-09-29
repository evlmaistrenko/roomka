import type { ErrorCode, InvalidInputReason } from "@/graphql/generated/enums"
import { i18n } from "@/lib/i18n"
import { en } from "@/locales/en"
import type { CombinedError } from "@urql/core"

// Clients branch on `extensions.code`, never on `message`, which is prose in
// the server's language. Nothing here ever shows it: a failure this client
// cannot name gets the generic line instead.

export function errorCodeOf(
	error: CombinedError | undefined,
): ErrorCode | undefined {
	return error?.graphQLErrors[0]?.extensions?.code as ErrorCode | undefined
}

export function hasErrorCode(
	error: CombinedError | undefined,
	code: ErrorCode,
): boolean {
	return (
		error?.graphQLErrors.some((item) => item.extensions?.code === code) ?? false
	)
}

export interface InvalidInput {
	// Outermost first: ["input", "password"].
	inputPath: string[]
	reason: InvalidInputReason
}

export function invalidInputOf(
	error: CombinedError | undefined,
): InvalidInput | undefined {
	const invalid = error?.graphQLErrors.find(
		(item) => item.extensions?.code === "INVALID_INPUT",
	)
	if (!invalid) return undefined
	return {
		inputPath: (invalid.extensions.inputPath as string[] | undefined) ?? [],
		reason: invalid.extensions.reason as InvalidInputReason,
	}
}

export function retryAfterSecondsOf(
	error: CombinedError | undefined,
): number | undefined {
	const limited = error?.graphQLErrors.find(
		(item) => item.extensions?.code === "RATE_LIMITED",
	)
	return limited?.extensions.retryAfterSeconds as number | undefined
}

// describeError is the line for a failure a form has no better words for, in
// the current language.
export function describeError(error: CombinedError): string {
	if (error.networkError) return i18n.t("errors.unreachable")
	const code = errorCodeOf(error)
	switch (code) {
		case "INVALID_INPUT":
			return describeInvalidInput(invalidInputOf(error)?.reason)
		case "UNAUTHENTICATED":
			return i18n.t("errors.unauthenticated")
		case "STEP_UP_REQUIRED":
			return i18n.t("errors.stepUpRequired")
		case "ACCESS_DENIED":
			return i18n.t("errors.accessDenied")
		case "LIMIT_REACHED":
			return i18n.t("errors.limitReached")
		case "RATE_LIMITED": {
			const seconds = retryAfterSecondsOf(error)
			return seconds
				? i18n.t("errors.rateLimited", { seconds })
				: i18n.t("errors.rateLimitedLater")
		}
		case "INTERNAL":
		case undefined:
			return i18n.t("errors.unknown")
		default:
			// A code the schema gained after this client was built.
			code satisfies never
			return i18n.t("errors.unknown")
	}
}

// describeInvalidInput names why an argument was rejected. A reason the
// schema gained after this client was built is described generically.
export function describeInvalidInput(reason: string | undefined): string {
	return isKnownReason(reason)
		? i18n.t(`invalidInput.${reason}`)
		: i18n.t("errors.invalidInput")
}

function isKnownReason(
	reason: string | undefined,
): reason is InvalidInputReason {
	return reason !== undefined && Object.hasOwn(en.invalidInput, reason)
}

export interface FormErrors<Field extends string> {
	fields: Partial<Record<Field, string>>
	// A failure no field owns: shown for the form as a whole.
	form?: string
}

// formErrorsOf places a failure on a form: a rejected argument under the field
// it names, anything else, including an argument the form has no field for,
// on the form as a whole.
export function formErrorsOf<Field extends string>(
	error: CombinedError | undefined,
	fields: readonly Field[],
): FormErrors<Field> {
	if (!error) return { fields: {} }
	const invalid = invalidInputOf(error)
	const field = fields.find((name) => name === invalid?.inputPath.at(-1))
	if (invalid && field) {
		return {
			fields: { [field]: describeInvalidInput(invalid.reason) } as Partial<
				Record<Field, string>
			>,
		}
	}
	return { fields: {}, form: describeError(error) }
}
