import * as React from "react"

import { cn } from "@common/lib/utils"

// A real <dialog> in modal mode, rather than a div wearing the role: it
// takes the focus, traps it, closes on Escape, paints its own backdrop,
// and gives the focus back to whatever opened it on close().
//
// Mounted means open. Every exit — Escape, the backdrop, a button —
// goes through close(), and onClose receives the returnValue, so a
// parent unmounts it in one place and the browser still restores focus.
const sides = {
	center: "m-auto max-h-[85vh] w-[calc(100%-2rem)] max-w-lg rounded-lg border p-6 shadow-lg",
	// The Sheet: a full-height panel on the right, with the page it
	// edits still visible beside it.
	right: "my-0 mr-0 ml-auto h-dvh max-h-dvh w-full max-w-xl border-l p-6 shadow-lg",
} as const

function Dialog({
	className,
	side = "center",
	onClose,
	children,
	...props
}: Omit<React.ComponentProps<"dialog">, "onClose" | "open"> & {
	side?: keyof typeof sides
	onClose: (returnValue: string) => void
}) {
	const ref = React.useRef<HTMLDialogElement>(null)

	React.useEffect(() => {
		const dialog = ref.current
		if (dialog !== null && !dialog.open) {
			dialog.showModal()
		}
	}, [])

	return (
		<dialog
			ref={ref}
			data-slot="dialog"
			className={cn(
				"overflow-y-auto bg-background text-foreground backdrop:bg-black/50",
				sides[side],
				className,
			)}
			onClose={(event) => {
				onClose(event.currentTarget.returnValue)
			}}
			onClick={(event) => {
				// A click on the dialog element itself, not on anything
				// inside it, is a click on the backdrop.
				if (event.target === event.currentTarget) {
					event.currentTarget.close()
				}
			}}
			{...props}
		>
			{children}
		</dialog>
	)
}

function DialogHeader({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="dialog-header"
			className={cn("mb-4 flex flex-col gap-1.5", className)}
			{...props}
		/>
	)
}

function DialogTitle({ className, ...props }: React.ComponentProps<"h2">) {
	return (
		<h2
			data-slot="dialog-title"
			className={cn("text-lg leading-none font-semibold", className)}
			{...props}
		/>
	)
}

function DialogDescription({ className, ...props }: React.ComponentProps<"p">) {
	return (
		<p
			data-slot="dialog-description"
			className={cn("text-sm text-muted-foreground", className)}
			{...props}
		/>
	)
}

function DialogFooter({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="dialog-footer"
			className={cn("mt-6 flex justify-end gap-2", className)}
			{...props}
		/>
	)
}

// Closes the <dialog> it sits in with the given returnValue.
function closeDialog(element: Element, value: string): void {
	element.closest("dialog")?.close(value)
}

export {
	Dialog,
	DialogHeader,
	DialogTitle,
	DialogDescription,
	DialogFooter,
	closeDialog,
}
