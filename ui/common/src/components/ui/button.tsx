import * as React from "react"

import { cn } from "@common/lib/utils"

const base =
	"inline-flex shrink-0 items-center justify-center rounded-md text-sm font-medium whitespace-nowrap transition-all outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50"

const variants = {
	default: "bg-primary text-primary-foreground hover:bg-primary/90",
	destructive:
		"bg-destructive text-white hover:bg-destructive/90 focus-visible:ring-destructive/20 dark:bg-destructive/60 dark:focus-visible:ring-destructive/40",
	outline:
		"border bg-background shadow-xs hover:bg-accent hover:text-accent-foreground dark:border-input dark:bg-input/30 dark:hover:bg-input/50",
	secondary: "bg-secondary text-secondary-foreground hover:bg-secondary/80",
	ghost: "hover:bg-accent hover:text-accent-foreground dark:hover:bg-accent/50",
	link: "text-primary underline-offset-4 hover:underline",
} as const

const sizes = {
	default: "h-9 gap-2 px-4",
	sm: "h-8 gap-1.5 px-3",
	xs: "h-7 gap-1 px-2 text-xs",
	icon: "size-9",
	"icon-sm": "size-7",
} as const

function Button({
	className,
	variant = "default",
	size,
	tooltip,
	style,
	onClick,
	...props
}: React.ComponentProps<"button"> & {
	variant?: keyof typeof variants
	size: keyof typeof sizes
	// Shown on hover and keyboard focus, in place of a native title,
	// which appears late and looks foreign. Read as the description
	// unless it only repeats the aria-label.
	tooltip?: string
}) {
	const uid = React.useId().replaceAll(":", "")
	const tip = React.useRef<HTMLDivElement>(null)
	const [open, setOpen] = React.useState(false)
	const describes = tooltip !== undefined && tooltip !== props["aria-label"]
	// The tip exists only while shown, bar one that is also the
	// description: a table of icon buttons would otherwise carry
	// thousands of hidden popovers through every style recalculation.
	const rendered = open || describes
	React.useLayoutEffect((): void => {
		if (open) tip.current?.showPopover()
		else if (tip.current?.matches(":popover-open"))
			tip.current.hidePopover()
	}, [open])
	const show = (): void => {
		setOpen(true)
	}
	const hide = (): void => {
		setOpen(false)
	}
	const button = (
		<button
			data-slot="button"
			data-variant={variant}
			data-size={size}
			className={cn(base, variants[variant], sizes[size], className)}
			style={
				tooltip && rendered
					? { anchorName: `--${uid}-tip`, ...style }
					: style
			}
			onClick={(event): void => {
				// A click usually opens something the tip would cover.
				hide()
				onClick?.(event)
			}}
			{...(tooltip && {
				onPointerEnter: show,
				onPointerLeave: hide,
				onFocus: (event: React.FocusEvent<HTMLButtonElement>): void => {
					if (event.currentTarget.matches(":focus-visible")) show()
				},
				onBlur: hide,
				"aria-describedby": describes ? `${uid}-tip` : undefined,
			})}
			{...props}
		/>
	)
	if (!tooltip) return button
	return (
		<>
			{button}
			{rendered && (
				<div
					ref={tip}
					id={`${uid}-tip`}
					popover="manual"
					role="tooltip"
					style={{ positionAnchor: `--${uid}-tip` }}
					className="anchored-tooltip pointer-events-none max-w-64 rounded-md bg-foreground px-2 py-1 text-xs text-background shadow-md"
				>
					{tooltip}
				</div>
			)}
		</>
	)
}

export { Button }
