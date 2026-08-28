import svelteParser from "svelte-eslint-parser"
import sveltePlugin from "eslint-plugin-svelte"
import tsParser from "@typescript-eslint/parser"
import tsPlugin from "@typescript-eslint/eslint-plugin"
import reactHooksPlugin from "eslint-plugin-react-hooks"
import globals from "globals"

const tsRules = {
	eqeqeq: ["error", "always"],
	indent: "off",
	"no-debugger": "error",
	"no-duplicate-imports": "error",
	"no-implied-eval": "error",
	"no-implicit-coercion": "error",
	"no-new-func": "error",
	"no-prototype-builtins": "error",
	"no-unreachable": "error",
	"no-unused-vars": "off",
	"no-useless-call": "error",
	"no-useless-concat": "error",
	"no-var": "error",
	"no-undef": "off",
	"prefer-const": "error",
	"prefer-object-spread": "error",
	"prefer-regex-literals": "error",
	"no-return-await": "error",
	"no-throw-literal": "error",
	"no-unneeded-ternary": "error",
	"no-useless-return": "error",
	"prefer-promise-reject-errors": "error",
	"require-await": "error",
	"prefer-template": "error",
	"no-extra-bind": "error",
	"no-useless-escape": "error",
	"no-self-compare": "error",

	"@typescript-eslint/await-thenable": "error",
	"@typescript-eslint/consistent-type-definitions": ["error", "interface"],
	"@typescript-eslint/consistent-type-imports": "error",
	"@typescript-eslint/explicit-function-return-type": [
		"error",
		{ allowExpressions: false },
	],
	"@typescript-eslint/no-empty-function": "error",
	"@typescript-eslint/no-explicit-any": "error",
	"@typescript-eslint/no-floating-promises": "error",
	"@typescript-eslint/no-inferrable-types": "error",
	"@typescript-eslint/no-misused-promises": "error",
	"@typescript-eslint/no-non-null-assertion": "error",
	"@typescript-eslint/no-unnecessary-condition": [
		"error",
		{ allowConstantLoopConditions: false },
	],
	"@typescript-eslint/no-unnecessary-type-assertion": "error",
	"@typescript-eslint/no-unused-expressions": "error",
	"@typescript-eslint/no-unused-vars": ["error", { argsIgnorePattern: "^_" }],
	"@typescript-eslint/no-var-requires": "error",
	"@typescript-eslint/strict-boolean-expressions": "error",
	"@typescript-eslint/no-duplicate-enum-values": "error",
	"@typescript-eslint/no-extra-non-null-assertion": "error",
	"@typescript-eslint/no-for-in-array": "error",
	"@typescript-eslint/no-unsafe-assignment": "error",
	"@typescript-eslint/no-unsafe-member-access": "error",
	"@typescript-eslint/no-unsafe-call": "error",
	"@typescript-eslint/no-unsafe-return": "error",
	"@typescript-eslint/restrict-template-expressions": "error",
	"@typescript-eslint/prefer-for-of": "error",
	"@typescript-eslint/prefer-optional-chain": "error",
	"@typescript-eslint/prefer-nullish-coalescing": "error",
	"@typescript-eslint/restrict-plus-operands": "error",
	"@typescript-eslint/unbound-method": "error",

	"no-console": "off",
	"no-alert": "off",
	"no-shadow": "error",
	"no-constant-condition": ["error", { checkLoops: true }],
	"no-empty": ["error", { allowEmptyCatch: false }],
	"no-extra-semi": "error",
	"no-multi-assign": "error",
	"no-nested-ternary": "error",
	"no-useless-catch": "error",
	"no-useless-rename": "error",
	"prefer-arrow-callback": "error",
	"prefer-rest-params": "error",
	"prefer-spread": "error",
	"@typescript-eslint/array-type": ["error", { default: "array-simple" }],
	"@typescript-eslint/explicit-member-accessibility": [
		"error",
		{ accessibility: "explicit" },
	],
	"@typescript-eslint/no-extraneous-class": "error",
	"@typescript-eslint/no-useless-empty-export": "error",
}

const svelteRules = {
	"svelte/indent": "off",
	"svelte/no-unused-svelte-ignore": "error",
	"svelte/no-at-html-tags": "off",
	"svelte/no-reactive-functions": "error",
	"svelte/no-reactive-literals": "error",
	"svelte/require-optimized-style-attribute": "error",
	"svelte/valid-compile": "error",
	"svelte/valid-each-key": "error",
}

export default [
	{
		files: ["**/src/**/*.ts", "**/src/**/*.tsx"],
		ignores: ["**/dist/**", "node_modules/**"],
		languageOptions: {
			parser: tsParser,
			parserOptions: {
				ecmaVersion: "latest",
				sourceType: "module",
				project: ["./tsconfig.json"],
				tsconfigRootDir: import.meta.dirname,
				warnOnUnsupportedTypeScriptVersion: true,
			},
			globals: {
				...globals.browser,
				...globals.es2021,
			},
		},
		plugins: {
			"@typescript-eslint": tsPlugin,
		},
		rules: tsRules,
	},
	{
		// The end-to-end tests: Node rather than browser globals, and
		// their own tsconfig, since they compile to real output.
		files: ["e2e/**/*.ts"],
		ignores: ["node_modules/**"],
		languageOptions: {
			parser: tsParser,
			parserOptions: {
				ecmaVersion: "latest",
				sourceType: "module",
				project: ["./e2e/tsconfig.json"],
				tsconfigRootDir: import.meta.dirname,
				warnOnUnsupportedTypeScriptVersion: true,
			},
			globals: {
				...globals.node,
				...globals.es2021,
			},
		},
		plugins: {
			"@typescript-eslint": tsPlugin,
		},
		rules: {
			...tsRules,
			// node:test's registration functions return promises that
			// are not meant to be awaited.
			"@typescript-eslint/no-floating-promises": [
				"error",
				{
					allowForKnownSafeCalls: [
						{
							from: "package",
							package: "node:test",
							name: [
								"after",
								"afterEach",
								"before",
								"beforeEach",
								"describe",
								"it",
							],
						},
					],
				},
			],
		},
	},
	{
		files: ["**/src/**/*.svelte"],
		ignores: ["**/dist/**", "node_modules/**"],
		languageOptions: {
			parser: svelteParser,
			parserOptions: {
				parser: tsParser,
				extraFileExtensions: [".svelte"],
				ecmaVersion: "latest",
				sourceType: "module",
				project: ["./tsconfig.json"],
				tsconfigRootDir: import.meta.dirname,
				warnOnUnsupportedTypeScriptVersion: true,
			},
			globals: {
				...globals.browser,
				...globals.es2021,
			},
		},
		plugins: {
			svelte: sveltePlugin,
			"@typescript-eslint": tsPlugin,
		},
		rules: {
			...tsRules,
			...svelteRules,
			// $bindable() props have to be declared with let, which the
			// core rule cannot know. The Svelte version understands the
			// runes and is otherwise identical.
			"prefer-const": "off",
			"svelte/prefer-const": "error",
		},
	},
	{
		// React components and the hooks they call. The block above
		// already supplies the parser and the shared TypeScript rules;
		// this one only adds what the core rules cannot see — the hook
		// rules, which are the whole of what React itself enforces.
		//
		// .ts as well as .tsx: a custom hook is a function, not markup,
		// and lives in a plain module. Scoped to .tsx alone, the rules
		// silently did not apply to it — and an eslint-disable naming
		// one of them became an error for a rule that was not loaded.
		files: ["**/src/**/*.ts", "**/src/**/*.tsx"],
		ignores: ["**/dist/**", "node_modules/**"],
		plugins: {
			"react-hooks": reactHooksPlugin,
		},
		rules: {
			"react-hooks/rules-of-hooks": "error",
			"react-hooks/exhaustive-deps": "error",
		},
	},
	{
		// Vendored shadcn primitives, written out by `shadcn add` and
		// replaced wholesale by the next one. Two rules are turned off
		// for them and nothing else: a return type on a function whose
		// body is a JSX literal states what the reader can already see,
		// and the generated fallbacks are `||` on purpose, since an
		// empty-string variant should fall through to the default the
		// same way undefined does.
		//
		// Scoped to this directory so the rules keep their full force on
		// every line anybody here actually writes.
		files: ["**/src/components/ui/**/*.tsx"],
		rules: {
			"@typescript-eslint/explicit-function-return-type": "off",
			"@typescript-eslint/prefer-nullish-coalescing": "off",
			"@typescript-eslint/strict-boolean-expressions": "off",
		},
	},
]
