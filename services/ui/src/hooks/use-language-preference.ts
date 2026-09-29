import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { graphql } from "@/graphql/generated"
import { VIEWER_SUBSCRIPTION } from "@/graphql/viewer"
import { useLiveSubscription } from "@/hooks/use-live-subscription"
import {
	LANGUAGE_PREFERENCE_KEY,
	type Language,
	isLanguage,
} from "@/lib/language"
import { useMutation } from "urql"

const SET_PREFERENCE_MUTATION = graphql(`
	mutation SetPreference($key: String!, $value: String) {
		setPreference(key: $key, input: { value: $value }) {
			key
		}
	}
`)

// The longest the page waits for the signed-in user's language before it
// shows itself in the one it guessed: a server that cannot be reached should
// not leave a spinner up for good.
const LANGUAGE_WAIT_MILLISECONDS = 5000

// useLanguagePreference follows the signed-in user's language: applied when it
// first arrives and again whenever it changes, from this tab or another one.
// Only a change applies it, so a language picked here a moment ago is not
// undone by the preference it is still on its way to replacing.
//
// It returns a promise that settles once the language is known: the
// preference applied, or none set and the guess kept, or no answer in time. A
// page suspends on it, so it never shows in one language and then flips to
// another.
export function useLanguagePreference(): Promise<void> {
	const { i18n } = useTranslation()
	const { data, error } = useLiveSubscription(VIEWER_SUBSCRIPTION)
	const [settled] = useState(deferred)
	const preferred = data?.me.preferences.find(
		(preference) => preference.key === LANGUAGE_PREFERENCE_KEY,
	)?.value
	const answered = data !== undefined || error !== undefined

	useEffect(() => {
		if (isLanguage(preferred)) {
			void i18n.changeLanguage(preferred).then(settled.resolve)
		} else if (answered) {
			settled.resolve()
		}
	}, [i18n, preferred, answered, settled])

	useEffect(() => {
		const timer = setTimeout(settled.resolve, LANGUAGE_WAIT_MILLISECONDS)
		return () => clearTimeout(timer)
	}, [settled])

	return settled.promise
}

function deferred(): { promise: Promise<void>; resolve: () => void } {
	let resolve = () => {}
	const promise = new Promise<void>((settle) => {
		resolve = () => settle()
	})
	return { promise, resolve }
}

// useChooseLanguage switches the page at once and stores the choice in the
// signed-in user's preferences, so their other browsers follow.
export function useChooseLanguage(): (language: Language) => void {
	const { i18n } = useTranslation()
	const [, setPreference] = useMutation(SET_PREFERENCE_MUTATION)
	return (language) => {
		void i18n.changeLanguage(language)
		void setPreference({ key: LANGUAGE_PREFERENCE_KEY, value: language })
	}
}
