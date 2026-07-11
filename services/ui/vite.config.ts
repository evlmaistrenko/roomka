import path from "node:path"

import react from "@vitejs/plugin-react"

import tailwindcss from "@tailwindcss/vite"
import { defineConfig, loadEnv } from "vite"

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
	// Env lives in the monorepo root .env.
	const envDir = path.resolve(import.meta.dirname, "../..")
	const env = loadEnv(mode, envDir, "ROOMKA_")
	return {
		plugins: [react(), tailwindcss()],
		envDir,
		// Expose only the two non-secret connection values to the client bundle —
		// whitelisted by exact name so nothing else in the ROOMKA_ namespace
		// (notably ROOMKA_ACCESS_SECRET) reaches import.meta.env.
		envPrefix: ["VITE_", "ROOMKA_HOSTNAME", "ROOMKA_WEB_TRANSPORT_PORT"],
		resolve: {
			alias: {
				"@": path.resolve(import.meta.dirname, "./src"),
			},
		},
		server: {
			// Reach the broadcast HTTP API same-origin, mirroring Caddy in the
			// container. ROOMKA_API_PORT is read here at config time only (loadEnv),
			// never exposed to the bundle.
			proxy: {
				"/api": `http://localhost:${env.ROOMKA_API_PORT}`,
			},
		},
	}
})
