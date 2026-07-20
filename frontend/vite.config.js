import { fileURLToPath, URL } from "node:url"

import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"

export default defineConfig({
	base: "/",
	plugins: [react(), tailwindcss()],
	resolve: {
		alias: {
			"@": fileURLToPath(new URL("./src", import.meta.url)),
		},
	},
	server: {
		proxy: {
			"/api": {
				target: process.env.CCA_API_TARGET ?? "http://127.0.0.1:8192",
				ws: true,
			},
		},
	},
})
