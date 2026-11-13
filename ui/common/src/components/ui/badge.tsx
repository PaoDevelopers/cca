import * as React from "react"

import { cn } from "@common/lib/utils"

const variants = {
	default: "border-transparent bg-primary text-primary-foreground",
	secondary: "border-transparent bg-secondary text-secondary-foreground",
	// The red darkened for its own tint in light mode, where plain
	// destructive on destructive/15 is 3.6:1; dark mode's red is light.
	destructive:
		"border-transparent bg-destructive/15 text-[color-mix(in_oklch,var(--destructive),black_25%)] dark:text-destructive",
	outline: "text-foreground",
} as const

function Badge({
	className,
	variant = "default",
	...props
}: React.ComponentProps<"span"> & { variant?: keyof typeof variants }) {
	return (
		<span
			data-slot="badge"
			className={cn(
				"inline-flex w-fit shrink-0 items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium whitespace-nowrap",
				variants[variant],
				className,
			)}
			{...props}
		/>
	)
}

export { Badge }
