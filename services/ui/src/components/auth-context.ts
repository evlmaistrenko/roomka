import { createContext } from "react"

export interface AuthState {
	// Something on the page failed with UNAUTHENTICATED; the sign-in page is
	// shown in its place until a sign-in completes.
	authenticationRequired: boolean
	// Called once a mutation has set the session cookie (login, setup).
	completeSignIn: () => void
	// Called once logout has cleared it.
	completeSignOut: () => void
}

export const AuthContext = createContext<AuthState | null>(null)
