import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { Navigate, RouterProvider, createBrowserRouter } from "react-router-dom"

import { AdminLayout } from "@/components/admin-layout"
import { AppLayout } from "@/components/app-layout"
import { AuthGate } from "@/components/auth-gate"
import { AuthProvider } from "@/components/auth-provider"
import { ThemeProvider } from "@/components/theme-provider"
import "@/lib/i18n"
import { ForgotPasswordPage } from "@/pages/forgot-password"
import { HomePage } from "@/pages/home"
import { NotFoundPage } from "@/pages/not-found"
import { SetPasswordPage } from "@/pages/set-password"
import { SettingsPage } from "@/pages/settings"
import { UsersPage } from "@/pages/users"

import "./index.css"

// Pages that need a session sit behind AuthGate. A page for visitors without
// one goes next to it rather than under it.
const router = createBrowserRouter([
	{ path: "set-password", element: <SetPasswordPage /> },
	{ path: "forgot-password", element: <ForgotPasswordPage /> },
	{
		element: <AuthGate />,
		children: [
			{
				element: <AppLayout />,
				children: [
					{ index: true, element: <HomePage /> },
					{
						path: "admin",
						element: <AdminLayout />,
						children: [
							{
								index: true,
								element: (
									<Navigate
										to="users"
										replace
									/>
								),
							},
							{ path: "users", element: <UsersPage /> },
						],
					},
					{ path: "settings", element: <SettingsPage /> },
					{ path: "*", element: <NotFoundPage /> },
				],
			},
		],
	},
])

createRoot(document.getElementById("root")!).render(
	<StrictMode>
		<ThemeProvider>
			<AuthProvider>
				<RouterProvider router={router} />
			</AuthProvider>
		</ThemeProvider>
	</StrictMode>,
)
