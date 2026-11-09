import * as React from "react"

import { cn } from "@common/lib/utils"

// ARIA tabs over panels that stay mounted, so a half-typed field in a
// tab that is not showing keeps its value. Arrow keys move between
// tabs, as the pattern requires.
function Tabs({
	label,
	tabs,
	className,
}: {
	label: string
	tabs: Array<{ id: string; title: string; content: React.ReactNode }>
	className?: string
}) {
	const uid = React.useId()
	const [current, setCurrent] = React.useState(tabs[0]?.id ?? "")
	const list = React.useRef<HTMLDivElement>(null)

	const move = (delta: number) => {
		const index = tabs.findIndex((tab) => tab.id === current)
		const next = tabs[(index + delta + tabs.length) % tabs.length]
		if (next !== undefined) {
			setCurrent(next.id)
			list.current
				?.querySelector<HTMLElement>(
					`#${CSS.escape(`${uid}-${next.id}`)}`,
				)
				?.focus()
		}
	}

	return (
		<div data-slot="tabs" className={cn("flex flex-col gap-3", className)}>
			<div
				ref={list}
				role="tablist"
				aria-label={label}
				className="inline-flex w-fit items-center rounded-lg bg-muted p-1 text-muted-foreground"
				onKeyDown={(event) => {
					if (event.key === "ArrowRight") move(1)
					if (event.key === "ArrowLeft") move(-1)
				}}
			>
				{tabs.map((tab) => (
					<button
						key={tab.id}
						id={`${uid}-${tab.id}`}
						type="button"
						role="tab"
						aria-selected={tab.id === current}
						aria-controls={`${uid}-${tab.id}-panel`}
						tabIndex={tab.id === current ? 0 : -1}
						className="rounded-md px-3 py-1 text-sm font-medium whitespace-nowrap transition-colors outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 aria-selected:bg-background aria-selected:text-foreground aria-selected:shadow-sm"
						onClick={() => {
							setCurrent(tab.id)
						}}
					>
						{tab.title}
					</button>
				))}
			</div>
			{tabs.map((tab) => (
				<div
					key={tab.id}
					id={`${uid}-${tab.id}-panel`}
					role="tabpanel"
					aria-labelledby={`${uid}-${tab.id}`}
					hidden={tab.id !== current}
				>
					{tab.content}
				</div>
			))}
		</div>
	)
}

export { Tabs }
