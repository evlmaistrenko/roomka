/**
 * @type {{
 * 	files: string[]
 * 	options: import("prettier").Config &
 * 		import("@trivago/prettier-plugin-sort-imports").PluginConfig &
 * 		Partial<import("prettier-plugin-jsdoc").Options>
 * }}
 */
const jsConfig = {
	files: ["*.js", ".jsx"],
	options: {
		parser: "babel",
		semi: false,
		useTabs: true,
		quoteProps: "consistent",
		trailingComma: "all",
		htmlWhitespaceSensitivity: "ignore",
		singleAttributePerLine: true,
		plugins: ["@trivago/prettier-plugin-sort-imports", "prettier-plugin-jsdoc"],
		importOrder: [
			"^dotenv",
			"^node:(.*)$",
			"react",
			"<THIRD_PARTY_MODULES>",
			"^[./]",
		],
		importOrderSeparation: true,
		importOrderSortSpecifiers: true,
		importOrderParserPlugins: ["jsx"],
		jsdocPrintWidth: 105,
	},
}

/** @type {import("prettier").Config} */
export default {
	endOfLine: "auto",
	overrides: [
		jsConfig,
		{
			...jsConfig,
			files: ["*.ts", "*.tsx"],
			options: {
				...jsConfig.options,
				parser: "typescript",
				importOrderParserPlugins: [
					...(jsConfig.options.importOrderParserPlugins
						? jsConfig.options.importOrderParserPlugins
						: []),
					"typescript",
				],
			},
		},
		{
			files: ["*.json"],
			options: {
				parser: "json",
				useTabs: true,
			},
		},
		{
			files: ["*.graphql", "*.gql"],
			options: {
				parser: "graphql",
				useTabs: true,
			},
		},
		{
			files: ["README.hbs", "*.md"],
			options: {
				parser: "markdown",
			},
		},
	],
}
