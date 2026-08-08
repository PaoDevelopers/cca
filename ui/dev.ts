import fs from "node:fs"
import { homedir } from "node:os"
import { join } from "node:path"
import type { ProxyOptions, ServerOptions } from "vite"

const backend = process.env["CCA_DEV_BACKEND"] ?? "https://cca.runxiyu.org"
const certFile =
	process.env["CCA_DEV_TLS_CERT"] ?? "/var/lib/tls/cca-leafchain.pem"
const keyFile = process.env["CCA_DEV_TLS_KEY"] ?? "/var/lib/tls/cca-privkey.pem"

export function devServer(port: number, proxyPaths: string[]): ServerOptions {
	const proxy: Record<string, ProxyOptions> = {}
	for (const path of proxyPaths) {
		proxy[path] = {
			target: backend,
			changeOrigin: true,
			secure: false, // We may use mkcert for development.
			ws: true,
		}
	}

	const server: ServerOptions = {
		host: true,
		port,
		strictPort: true,
		allowedHosts: true,
		proxy,
	}

	if (fs.existsSync(certFile) && fs.existsSync(keyFile)) {
		server.https = {
			cert: fs.readFileSync(certFile),
			key: fs.readFileSync(keyFile),
		}
	}

	return server
}
