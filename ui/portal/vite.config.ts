import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import { fileURLToPath } from "node:url"
import { defineConfig } from "vite"
import { devServer } from "../dev"

// The page at the root: a door to each of the two panels, and part of
// neither. It builds on its own so that it stays that way — nothing
// here is served from under /student/ or /admin/, and neither panel is
// on the critical path of the page somebody sees first.
export default defineConfig({
	root: fileURLToPath(new URL(".", import.meta.url)),
	base: "/",
	plugins: [react(), tailwindcss()],
	resolve: {
		alias: {
			"@common": fileURLToPath(new URL("../common/src", import.meta.url)),
		},
	},
	build: {
		minify: "esbuild",
		modulePreload: { polyfill: false },
		outDir: "dist",
		emptyOutDir: true,
	},
	server: devServer(5175, ["/api/session"]),
})
