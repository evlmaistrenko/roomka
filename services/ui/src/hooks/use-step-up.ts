import { useCallback, useContext, useState } from "react"

import { StepUpContext, type StepUpState } from "@/components/step-up-context"
import { hasErrorCode } from "@/lib/graphql-errors"
import type { AnyVariables, DocumentInput, OperationResult } from "@urql/core"
import { type UseMutationState, useMutation } from "urql"

export function useStepUp(): StepUpState {
	const context = useContext(StepUpContext)
	if (!context) throw new Error("useStepUp must be used within StepUpProvider")
	return context
}

// useStepUpMutation runs a mutation that may require a raised session. When the
// server answers STEP_UP_REQUIRED it asks for the password and, once the
// session is raised, retries unchanged, as the schema says to. If the user
// gives up, the STEP_UP_REQUIRED answer is returned as it came.
//
// The state covers the whole exchange, not the last request in it: it stays
// fetching while the password is asked for, and the STEP_UP_REQUIRED that
// started the exchange is not shown as a failure while it is being resolved.
export function useStepUpMutation<
	Data,
	Variables extends AnyVariables = AnyVariables,
>(
	query: DocumentInput<Data, Variables>,
): [
	UseMutationState<Data, Variables>,
	(variables: Variables) => Promise<OperationResult<Data, Variables>>,
] {
	const [state, execute] = useMutation<Data, Variables>(query)
	const { confirmPassword } = useStepUp()
	const [running, setRunning] = useState(false)
	const run = useCallback(
		async (variables: Variables) => {
			setRunning(true)
			try {
				let response = await execute(variables)
				while (hasErrorCode(response.error, "STEP_UP_REQUIRED")) {
					if (!(await confirmPassword())) break
					response = await execute(variables)
				}
				return response
			} finally {
				setRunning(false)
			}
		},
		[execute, confirmPassword],
	)
	return [running ? { ...state, fetching: true, error: undefined } : state, run]
}
