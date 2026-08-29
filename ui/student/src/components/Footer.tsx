import type { ReactElement } from "react"

export function Footer(): ReactElement {
	return (
		<footer className="mt-6 border-t px-4 py-3 text-xs text-muted-foreground sm:px-6">
			<a
				className="underline underline-offset-2 hover:text-foreground"
				href="https://github.com/PaoDevelopers/cca"
			>
				YK Pao School CCAs
			</a>{" "}
			is licensed under{" "}
			<a
				className="underline underline-offset-2 hover:text-foreground"
				href="https://spdx.org/licenses/AGPL-3.0-only.html"
			>
				GNU Affero General Public License v3.0 only
			</a>
			.
		</footer>
	)
}
