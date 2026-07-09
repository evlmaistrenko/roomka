import path from "node:path"

import react from "@vitejs/plugin-react"

import tailwindcss from "@tailwindcss/vite"
import { defineConfig } from "vite"

// https://vite.dev/config/
export default defineConfig({
	plugins: [react(), tailwindcss()],
	// Env lives in the monorepo root .env. Expose only the shared, non-secret
	// ROOMKA_PUBLIC_* vars to the client bundle — never the ROOMKA_BROADCAST_*
	// secrets, which stay out of import.meta.env.
	envDir: path.resolve(import.meta.dirname, "../.."),
	envPrefix: ["VITE_", "ROOMKA_PUBLIC_"],
	resolve: {
		alias: {
			"@": path.resolve(import.meta.dirname, "./src"),
		},
	},
})
