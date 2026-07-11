import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { RouterProvider, createMemoryRouter } from "react-router-dom"

import App from "@/App.tsx"
import { ThemeProvider } from "@/components/theme-provider"
import { loadSettings } from "@/lib/settings"
import { ImportSettings } from "@/pages/import-settings"

import "./index.css"

// In-memory routing (the URL never changes): on startup show the settings
// import page unless a valid settings file was already imported (persisted in
// localStorage).
const router = createMemoryRouter(
	[
		{ path: "/", element: <App /> },
		{ path: "/import", element: <ImportSettings /> },
	],
	{ initialEntries: [loadSettings() ? "/" : "/import"] },
)

createRoot(document.getElementById("root")!).render(
	<StrictMode>
		<ThemeProvider>
			<RouterProvider router={router} />
		</ThemeProvider>
	</StrictMode>,
)
