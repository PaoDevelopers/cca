import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import { fileURLToPath } from "node:url"
import { defineConfig } from "vite"
import { devServer } from "./dev"

// The two React frontends, built together so that they share one copy
// of React.
//
// They are separate pages, not one app with two views: the portal at
// the root chooses between the areas and the student panel is one of
// them. Neither directory is inside the other, and this config is in
// neither of them. What they share is the runtime — 62 kB gzipped that
// used to be downloaded twice, once per build, under two different
// hashes that could not be cached for each other.
//
// The price is that one build has one `base`, so both pages take "/"
// and the panel's assets moved from /student/assets/ to /assets/. The
// SPA still lives at /student/; only where its script comes from
// changed. The Svelte admin panel keeps its own build.
export default defineConfig({
	root: fileURLToPath(new URL(".", import.meta.url)),
	base: "/",
	// Directory URLs resolve to the index.html in them rather than
	// falling back to a root index.html. Neither page routes on the
	// URL, so there is no history fallback to want.
	appType: "mpa",
	plugins: [react(), tailwindcss()],
	resolve: {
		alias: {
			"@common": fileURLToPath(new URL("./common/src", import.meta.url)),
			// The panel's own alias, from when it was its own build. The
			// portal does not use it.
			"@": fileURLToPath(new URL("./student/src", import.meta.url)),
		},
	},
	build: {
		minify: "esbuild",
		modulePreload: { polyfill: false },
		outDir: "dist",
		emptyOutDir: true,
		rollupOptions: {
			input: {
				portal: fileURLToPath(
					new URL("./portal/index.html", import.meta.url),
				),
				student: fileURLToPath(
					new URL("./student/index.html", import.meta.url),
				),
			},
		},
	},
	server: devServer(5173, [
		"/student/api",
		"/student/logout",
		"/api/session",
	]),
})
