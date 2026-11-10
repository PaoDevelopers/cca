import * as React from "react"

import { cn } from "@common/lib/utils"

const variants = {
	default: "border-transparent bg-primary text-primary-foreground",
	secondary: "border-transparent bg-secondary text-secondary-foreground",
	destructive: "border-transparent bg-destructive/15 text-destructive",
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
