// The panel's drawn icons: one 16-unit grid and one stroke weight, so
// they read as a set beside each other in a row's actions.
import type { ReactElement } from "react"

// A drawn trash can, the mark for every delete in the panel.
export function TrashIcon(): ReactElement {
	return (
		<svg
			aria-hidden="true"
			viewBox="0 0 16 16"
			className="size-[1.125rem]"
			fill="none"
			stroke="currentColor"
			strokeWidth="1.5"
			strokeLinecap="round"
			strokeLinejoin="round"
		>
			<path d="M2.5 4.5h11M6.5 4.5V3a1 1 0 0 1 1-1h1a1 1 0 0 1 1 1v1.5M4 4.5l.7 8.6a1 1 0 0 0 1 .9h4.6a1 1 0 0 0 1-.9l.7-8.6M6.75 7v4.5M9.25 7v4.5" />
		</svg>
	)
}

// A pencil, for opening an editor.
export function PencilIcon(): ReactElement {
	return (
		<svg
			aria-hidden="true"
			viewBox="0 0 16 16"
			className="size-[1.125rem]"
			fill="none"
			stroke="currentColor"
			strokeWidth="1.5"
			strokeLinecap="round"
			strokeLinejoin="round"
		>
			<path d="M10.5 2.5l3 3L5.5 13.5 2 14l.5-3.5 8-8zM9 4l3 3" />
		</svg>
	)
}
