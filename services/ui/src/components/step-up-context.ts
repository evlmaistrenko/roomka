import { createContext } from "react"

export interface StepUpState {
	// Asks for the password again and raises the session with it. Resolves true
	// once the session is raised, false if the user gave up.
	confirmPassword: () => Promise<boolean>
}

export const StepUpContext = createContext<StepUpState | null>(null)
