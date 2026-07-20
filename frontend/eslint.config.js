import tsParser from "@typescript-eslint/parser"
import tsPlugin from "@typescript-eslint/eslint-plugin"
import prettierPlugin from "eslint-plugin-prettier"
import reactHooks from "eslint-plugin-react-hooks"
import reactRefresh from "eslint-plugin-react-refresh"
import globals from "globals"

export default [
	{
		ignores: ["dist/**", "node_modules/**"],
	},
	{
		files: ["src/**/*.{ts,tsx}"],
		languageOptions: {
			parser: tsParser,
			parserOptions: {
				ecmaVersion: "latest",
				sourceType: "module",
				project: ["./tsconfig.json"],
				tsconfigRootDir: process.cwd(),
			},
			globals: {
				...globals.browser,
				...globals.es2021,
			},
		},
		plugins: {
			"@typescript-eslint": tsPlugin,
			prettier: prettierPlugin,
			"react-hooks": reactHooks,
			"react-refresh": reactRefresh,
		},
		rules: {
			...reactHooks.configs.recommended.rules,
			"react-refresh/only-export-components": [
				"warn",
				{ allowConstantExport: true },
			],
			"prettier/prettier": "error",
			"@typescript-eslint/consistent-type-imports": "error",
			"@typescript-eslint/no-explicit-any": "error",
			"@typescript-eslint/no-floating-promises": "error",
			"@typescript-eslint/no-misused-promises": "error",
			"@typescript-eslint/no-unused-vars": [
				"error",
				{ argsIgnorePattern: "^_" },
			],
			"no-console": ["warn", { allow: ["warn", "error"] }],
			"no-debugger": "error",
			"no-duplicate-imports": "error",
			"prefer-const": "error",
		},
	},
	{
		files: ["src/components/ui/**/*.{ts,tsx}"],
		rules: {
			// Registry components intentionally export their shared variants too.
			"react-refresh/only-export-components": "off",
		},
	},
]
