import vueParser from 'vue-eslint-parser'
import vuePlugin from 'eslint-plugin-vue'
import tsParser from '@typescript-eslint/parser'
import tsPlugin from '@typescript-eslint/eslint-plugin'
import prettierPlugin from 'eslint-plugin-prettier'
import globals from 'globals'

export default [
	{
		files: ['src/**/*.{ts,vue}'],
		ignores: ['dist/**', 'node_modules/**'],
		languageOptions: {
			parser: vueParser,
			parserOptions: {
				parser: tsParser,
				extraFileExtensions: ['.vue'],
				ecmaVersion: 'latest',
				sourceType: 'module',
				project: ['./tsconfig.json'],
				tsconfigRootDir: process.cwd(),
			},
			globals: {
				...globals.browser,
				...globals.es2021,
			},
		},
		plugins: {
			vue: vuePlugin,
			'@typescript-eslint': tsPlugin,
			prettier: prettierPlugin,
		},
		rules: {
			eqeqeq: ['error', 'always'],
			indent: 'off',
			'no-debugger': 'error',
			'no-duplicate-imports': 'error',
			'no-implied-eval': 'error',
			'no-implicit-coercion': 'error',
			'no-new-func': 'error',
			'no-prototype-builtins': 'error',
			'no-unreachable': 'error',
			'no-unused-vars': 'error',
			'no-useless-call': 'error',
			'no-useless-concat': 'error',
			'no-var': 'error',
			'no-undef': 'error',
			'prefer-const': 'error',
			'prefer-object-spread': 'error',
			'prefer-regex-literals': 'error',
			'prettier/prettier': ['error'],

			'@typescript-eslint/await-thenable': 'error',
			'@typescript-eslint/consistent-type-definitions': [
				'error',
				'interface',
			],
			'@typescript-eslint/consistent-type-imports': 'error',
			'@typescript-eslint/explicit-function-return-type': [
				'error',
				{ allowExpressions: false },
			],
			'@typescript-eslint/no-empty-function': 'error',
			'@typescript-eslint/no-explicit-any': 'error',
			'@typescript-eslint/no-floating-promises': 'error',
			'@typescript-eslint/no-inferrable-types': 'error',
			'@typescript-eslint/no-misused-promises': 'error',
			'@typescript-eslint/no-non-null-assertion': 'error',
			'@typescript-eslint/no-unnecessary-condition': [
				'error',
				{ allowConstantLoopConditions: false },
			],
			'@typescript-eslint/no-unnecessary-type-assertion': 'error',
			'@typescript-eslint/no-unused-expressions': 'error',
			'@typescript-eslint/no-unused-vars': [
				'error',
				{ argsIgnorePattern: '^_' },
			],
			'@typescript-eslint/no-var-requires': 'error',
			'@typescript-eslint/strict-boolean-expressions': 'error',

			'vue/html-indent': 'off',
			'vue/multi-word-component-names': 'off',
			'vue/no-async-in-computed-properties': 'error',
			'vue/no-mutating-props': 'error',
			'vue/no-ref-as-operand': 'error',
			'vue/no-side-effects-in-computed-properties': 'error',
			'vue/no-unused-components': 'error',
			'vue/no-unused-properties': [
				'error',
				{ groups: ['props', 'data', 'computed', 'methods'] },
			],
			'vue/no-unused-vars': 'error',
			'vue/no-v-html': 'off',
			'vue/no-v-text': 'error',
			'vue/require-default-prop': 'error',
			'vue/require-explicit-emits': 'error',
			'vue/require-prop-types': 'error',
			'vue/script-indent': 'off',
		},
	},
	{
		files: ['src/**/*.js'],
		ignores: ['dist/**', 'node_modules/**'],
		languageOptions: {
			ecmaVersion: 'latest',
			sourceType: 'module',
			globals: {
				...globals.browser,
				...globals.es2021,
			},
		},
		plugins: {
			prettier: prettierPlugin,
		},
		rules: {
			eqeqeq: ['error', 'always'],
			'no-debugger': 'error',
			'no-duplicate-imports': 'error',
			'no-unreachable': 'error',
			'no-unused-vars': 'error',
			'no-undef': 'error',
			'no-var': 'error',
			'prefer-const': 'error',
			'prettier/prettier': ['error'],
		},
	},
]
