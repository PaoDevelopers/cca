import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import { fileURLToPath } from "node:url"
import { defineConfig } from "vite"
import { devServer } from "../dev"

export default defineConfig({
	root: fileURLToPath(new URL(".", import.meta.url)),
	base: "/student/",
	plugins: [react(), tailwindcss()],
	resolve: {
		alias: {
			"@common": fileURLToPath(new URL("../common/src", import.meta.url)),
			"@": fileURLToPath(new URL("./src", import.meta.url)),
		},
	},
	build: {
		minify: "esbuild",
		modulePreload: { polyfill: false },
		outDir: "dist",
		emptyOutDir: true,
	},
	server: devServer(5173, ["/student/api", "/student/logout"]),
})
