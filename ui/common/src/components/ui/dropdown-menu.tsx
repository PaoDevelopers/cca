import * as React from "react"

import { cn } from "@common/lib/utils"

// A trigger and a native popover anchored under it. The popover API
// gives light dismiss and Escape; the items are ordinary buttons, so
// Tab reaches them without a roving focus to maintain.
function DropdownMenu({
	trigger,
	children,
	className,
}: {
	trigger: (props: {
		popoverTarget: string
		style: React.CSSProperties
	}) => React.ReactNode
	children: React.ReactNode
	className?: string
}) {
	const uid = React.useId().replaceAll(":", "")
	return (
		<>
			{trigger({
				popoverTarget: `${uid}-menu`,
				style: { anchorName: `--${uid}-menu` },
			})}
			<div
				id={`${uid}-menu`}
				popover="auto"
				data-slot="dropdown-menu"
				style={{ positionAnchor: `--${uid}-menu` }}
				className={cn(
					"anchored-menu min-w-48 rounded-md border bg-popover p-1 text-popover-foreground shadow-md",
					className,
				)}
				onClick={(event) => {
					// Choosing an item closes the menu; clicks on the
					// menu's own padding do not.
					if (event.target !== event.currentTarget) {
						event.currentTarget.hidePopover()
					}
				}}
			>
				{children}
			</div>
		</>
	)
}

const itemClass =
	"flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm outline-none hover:bg-accent focus-visible:bg-accent disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0"

function DropdownMenuItem({
	className,
	...props
}: React.ComponentProps<"button">) {
	return (
		<button
			type="button"
			data-slot="dropdown-menu-item"
			className={cn(itemClass, className)}
			{...props}
		/>
	)
}

// A link that looks like an item: an export is a download, and a
// download is a navigation, which a button would have to fake.
function DropdownMenuLink({ className, ...props }: React.ComponentProps<"a">) {
	return (
		<a
			data-slot="dropdown-menu-item"
			className={cn(itemClass, className)}
			{...props}
		/>
	)
}

function DropdownMenuLabel({
	className,
	...props
}: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="dropdown-menu-label"
			className={cn(
				"px-2 py-1.5 text-xs font-medium text-muted-foreground",
				className,
			)}
			{...props}
		/>
	)
}

export { DropdownMenu, DropdownMenuItem, DropdownMenuLink, DropdownMenuLabel }
