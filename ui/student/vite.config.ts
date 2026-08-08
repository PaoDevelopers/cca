import { svelte } from "@sveltejs/vite-plugin-svelte"
import { fileURLToPath } from "node:url"
import { defineConfig } from "vite"
import { devServer } from "../dev"

export default defineConfig({
	root: fileURLToPath(new URL(".", import.meta.url)),
	base: "/student/",
	plugins: [svelte()],
	resolve: {
		alias: {
			"@common": fileURLToPath(new URL("../common/src", import.meta.url)),
		},
	},
	build: {
		outDir: "dist",
		emptyOutDir: true,
	},
	server: devServer(5173, ["/student/api", "/student/logout"]),
})
