import { useContext } from "react"

import { AuthContext, type AuthState } from "@/components/auth-context"

export function useAuth(): AuthState {
	const context = useContext(AuthContext)
	if (!context) throw new Error("useAuth must be used within AuthProvider")
	return context
}
