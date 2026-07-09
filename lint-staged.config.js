/**
 * Lint-staged runs from the repo root. For each glob it either passes the staged file paths to the
 * command (string form) or runs a fixed command (function form, used for tools that operate on a whole
 * package/module rather than file lists).
 *
 * @type {import("lint-staged").Configuration}
 */
export default {
	// UI: format the staged files, then lint the whole package (its eslint flat
	// config resolves from the package dir, so run it via the workspace).
	"services/ui/**/*.{ts,tsx,js,jsx}": [
		"prettier --write",
		() => "npm run lint --workspace services/ui",
	],

	// Go relay: gofmt the staged files, then vet the module.
	"services/broadcast/**/*.go": [
		"gofmt -w",
		() => "go -C services/broadcast vet ./...",
	],

	// graphql service + everything else Prettier can format.
	"services/graphql/**/*.{js,ts,json,graphql,gql}": "prettier --write",
	"*.{js,ts,json,md,yml,yaml}": "prettier --write",
}
