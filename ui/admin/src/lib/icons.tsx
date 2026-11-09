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

// A calendar, for a weekly schedule.
export function CalendarIcon(): ReactElement {
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
			<path d="M3 3.5h10a1 1 0 0 1 1 1V13a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V4.5a1 1 0 0 1 1-1zM2 7h12M5.5 2v3M10.5 2v3" />
		</svg>
	)
}

// A list, for the enrollments a row leads to.
export function ListIcon(): ReactElement {
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
			<path d="M6 4h8M6 8h8M6 12h8M2 4h1.5M2 8h1.5M2 12h1.5" />
		</svg>
	)
}

// An arrow into a door, for signing in as someone.
export function SignInIcon(): ReactElement {
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
			<path d="M9.5 2.5H12a1.5 1.5 0 0 1 1.5 1.5v8a1.5 1.5 0 0 1-1.5 1.5H9.5M2.5 8h7M7 5l3 3-3 3" />
		</svg>
	)
}
