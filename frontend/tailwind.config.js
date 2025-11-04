/** @type {import('tailwindcss').Config} */
export default {
	content: ["./index.html", "./src/**/*.{vue,js,ts,jsx,tsx}"],
	theme: {
		extend: {
			colors: {
				primary: {
					DEFAULT: "rgb(var(--cca-primary) / <alpha-value>)",
					soft: "rgb(var(--cca-primary-soft) / <alpha-value>)",
				},
				success: {
					DEFAULT: "rgb(var(--cca-success) / <alpha-value>)",
					soft: "rgb(var(--cca-success-soft) / <alpha-value>)",
				},
				danger: {
					DEFAULT: "rgb(var(--cca-danger) / <alpha-value>)",
					soft: "rgb(var(--cca-danger-soft) / <alpha-value>)",
				},
				warning: {
					DEFAULT: "rgb(var(--cca-warning) / <alpha-value>)",
					soft: "rgb(var(--cca-warning-soft) / <alpha-value>)",
				},
				info: {
					DEFAULT: "rgb(var(--cca-info) / <alpha-value>)",
					soft: "rgb(var(--cca-info-soft) / <alpha-value>)",
				},
				surface: {
					DEFAULT: "rgb(var(--cca-surface))",
					alt: "rgb(var(--cca-surface-alt))",
				},
				border: {
					DEFAULT: "rgb(var(--cca-border) / <alpha-value>)",
				},
				ink: {
					DEFAULT: "rgb(var(--cca-text) / <alpha-value>)",
					muted: "rgb(var(--cca-text-muted) / <alpha-value>)",
				},
				backdrop: "rgb(var(--cca-backdrop) / <alpha-value>)",
			},
			boxShadow: {
				focus: "0 0 0 3px rgba(var(--cca-focus-ring) / 0.35)",
			},
		},
	},
	plugins: [require("daisyui")],
	daisyui: {
		themes: [
			{
				ccaLight: {
					primary: "#5bae31",
					"primary-content": "#ffffff",
					accent: "#5bae31",
					neutral: "#1f2933",
					"base-100": "#ffffff",
					"base-200": "#f7faf7",
					"base-300": "#eef4ef",
					success: "#5bae31",
					warning: "#875413",
					error: "#9b1c1c",
					info: "#5bae31",
				},
			},
		],
	},
}
