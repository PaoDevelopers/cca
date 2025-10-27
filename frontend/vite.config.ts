import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import fs from 'fs'

// https://vite.dev/config/
export default defineConfig({
	plugins: [svelte()],
	server: {
		host: 'localhost',
		port: '443',
		https: {
			key: fs.readFileSync("/home/runxiyu/.local/share/secrets/cca.r.o-key.pem"),
			cert: fs.readFileSync("/home/runxiyu/.local/share/secrets/cca.r.o-cert.pem"),
		},
		proxy: {
			'/auth': {
				target: 'http://localhost:8192',
				changeOrigin: true,
				secure: false,
				ws: true,
			},
			'/api': {
				target: 'http://localhost:8192',
				changeOrigin: true,
				secure: false,
				ws: true,
			},
		},
	},
})
