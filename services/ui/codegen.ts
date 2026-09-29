import type { CodegenConfig } from "@graphql-codegen/cli"

// Types for the operations the UI writes with `graphql()`, checked against the
// control server's SDL. The output is committed, like every generated file in
// the repository, so a fresh checkout type-checks without running this.
const config: CodegenConfig = {
	schema: "../../packages/control-specs/graphql/*.graphqls",
	documents: ["src/**/*.{ts,tsx}", "!src/graphql/generated/**"],
	ignoreNoDocuments: true,
	generates: {
		"src/graphql/generated/": {
			preset: "client",
			presetConfig: {
				// A component reads whatever its own document selects; masking
				// would only add an unwrap step to every fragment.
				fragmentMasking: false,
			},
			config: {
				// erasableSyntaxOnly rules out TypeScript enums.
				enumType: "string-literal",
				useTypeImports: true,
				scalars: {
					DateTime: "string",
					Void: "null",
				},
			},
		},
		// Every enum of the schema, not only those an operation selects:
		// ErrorCode and InvalidInputReason arrive in `extensions`, which no
		// document names.
		"src/graphql/generated/enums.ts": {
			plugins: ["typescript"],
			config: {
				onlyEnums: true,
				enumsAsTypes: true,
			},
		},
	},
}

export default config
