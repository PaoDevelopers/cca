/** @type {import('tailwindcss').Config} */
export default {
	content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
	theme: {
		extend: {
			colors: {
				primary: '#5bae31',
			},
		},
	},
	plugins: [require('daisyui')],
	daisyui: {
		themes: ['light'],
		darkTheme: false,
	},
}
