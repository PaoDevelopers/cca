import { useEffect, useRef, type ReactElement } from "react"

interface Props {
	message: string | null
	ondismiss: () => void
}

// An error popover, anchored to whatever the failing action was
// triggered from. Light-dismisses through ondismiss.
//
// The failing action's button is disabled while the request runs, which
// drops focus to <body> before the error arrives — so the focused
// element is useless as an anchor, and the last interacted-with element
// is recorded instead. Capture phase, so it is recorded even when a
// handler further down stops propagation.
export function ErrorPopup({ message, ondismiss }: Props): ReactElement {
	const ref = useRef<HTMLDivElement>(null)
	const lastInteraction = useRef<HTMLElement | null>(null)

	useEffect((): (() => void) => {
		const remember = (event: Event): void => {
			if (event.target instanceof HTMLElement) {
				lastInteraction.current = event.target
			}
		}
		window.addEventListener("pointerdown", remember, true)
		window.addEventListener("keydown", remember, true)
		return (): void => {
			window.removeEventListener("pointerdown", remember, true)
			window.removeEventListener("keydown", remember, true)
		}
	}, [])

	useEffect((): (() => void) | undefined => {
		const el = ref.current
		if (el === null) {
			return undefined
		}
		if (message === null) {
			if (el.matches(":popover-open")) {
				el.hidePopover()
			}
			return undefined
		}

		const source =
			(lastInteraction.current?.isConnected ?? false)
				? lastInteraction.current
				: null
		if (source !== null) {
			source.style.setProperty("anchor-name", "--error-source")
			el.classList.add("anchored")
		} else {
			el.classList.remove("anchored")
		}

		// One message replacing another re-runs this with the popover
		// still open, and showPopover() throws on an open popover.
		if (!el.matches(":popover-open")) {
			el.showPopover()
		}

		return (): void => {
			source?.style.removeProperty("anchor-name")
		}
	}, [message])

	return (
		<div
			ref={ref}
			popover="auto"
			role="alert"
			className="error-popover"
			onToggle={(event): void => {
				if (
					(event as unknown as { newState: string }).newState ===
						"closed" &&
					message !== null
				) {
					ondismiss()
				}
			}}
		>
			{message}
		</div>
	)
}
