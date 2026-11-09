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
	...props
}: React.ComponentProps<"button"> & {
	variant?: keyof typeof variants
	size: keyof typeof sizes
}) {
	return (
		<button
			data-slot="button"
			data-variant={variant}
			data-size={size}
			className={cn(base, variants[variant], sizes[size], className)}
			{...props}
		/>
	)
}

export { Button }
