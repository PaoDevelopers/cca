import * as React from "react"

import { cn } from "@common/lib/utils"

// Success notices. Errors do not come through here: they are anchored
// to the control that failed (ErrorPopup), which a toast in the corner
// cannot point at.
//
// A module-level list rather than a context, so the data layer, which
// is not a component, can announce too.
interface Toast {
	id: number
	message: string
}

let toasts: Toast[] = []
let nextID = 0
const listeners = new Set<() => void>()

function emit(): void {
	for (const listener of listeners) listener()
}

function toast(message: string): void {
	const id = nextID++
	toasts = [...toasts, { id, message }]
	emit()
	setTimeout(() => {
		toasts = toasts.filter((t) => t.id !== id)
		emit()
	}, 4000)
}

function subscribe(listener: () => void): () => void {
	listeners.add(listener)
	return () => listeners.delete(listener)
}

// One live region, always in the document: a region inserted together
// with its first message is not announced by every screen reader.
function Toaster({ className }: { className?: string }) {
	const current = React.useSyncExternalStore(subscribe, () => toasts)
	return (
		<div
			role="status"
			aria-live="polite"
			className={cn(
				"pointer-events-none fixed right-4 bottom-4 z-50 flex flex-col gap-2",
				className,
			)}
		>
			{current.map((t) => (
				<div
					key={t.id}
					className="rounded-lg border bg-popover px-4 py-3 text-sm text-popover-foreground shadow-lg"
				>
					{t.message}
				</div>
			))}
		</div>
	)
}

export { Toaster, toast }
