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
			'no-undef': 'error',
			'no-unused-vars': 'off',
			'@typescript-eslint/no-unused-vars': [
				'warn',
				{ argsIgnorePattern: '^_' },
			],
			'no-unreachable': 'error',
			'no-console': ['warn', { allow: ['warn', 'error'] }],
			'no-debugger': 'error',
			eqeqeq: ['error', 'always'],
			'no-duplicate-imports': 'error',
			'no-var': 'error',
			'prefer-const': 'warn',

			'@typescript-eslint/no-explicit-any': 'error',
			'@typescript-eslint/consistent-type-imports': 'error',
			'@typescript-eslint/no-non-null-assertion': 'error',
			'@typescript-eslint/no-inferrable-types': 'error',
			'@typescript-eslint/no-empty-function': 'error',
			'@typescript-eslint/no-misused-promises': 'error',
			'@typescript-eslint/await-thenable': 'error',
			'@typescript-eslint/no-floating-promises': 'error',

			'vue/multi-word-component-names': 'off',
			'vue/no-unused-components': 'error',
			'vue/no-mutating-props': 'error',
			'vue/no-v-html': 'off',
			'vue/no-unused-vars': 'error',

			'vue/html-indent': 'off',
			'vue/script-indent': 'off',
			indent: 'off',
			'prettier/prettier': ['error'],
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
			'no-unused-vars': 'warn',
			'no-undef': 'error',
			'no-unreachable': 'error',
			'no-console': ['warn', { allow: ['warn', 'error'] }],
			'no-debugger': 'error',
			eqeqeq: ['error', 'always'],
			'no-duplicate-imports': 'error',
			'no-var': 'error',
			'prefer-const': 'warn',
			'prettier/prettier': ['warn'],
		},
	},
]
