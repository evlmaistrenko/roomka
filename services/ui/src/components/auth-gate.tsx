import { Outlet } from "react-router-dom"

import { useAuth } from "@/hooks/use-auth"
import { SignInPage } from "@/pages/sign-in"

// AuthGate renders its routes until one of them fails with UNAUTHENTICATED,
// and the sign-in page in their place from then on. Unmounting the routes
// also ends their subscriptions, which belong to the old connection anyway.
export function AuthGate() {
	const { authenticationRequired } = useAuth()
	return authenticationRequired ? <SignInPage /> : <Outlet />
}
