import type { ReactElement } from "react"
import { Button } from "@/components/ui/button"

interface Props {
	view: "cards" | "table"
	onview: (view: "cards" | "table") => void
}

const views = [
	{ key: "cards", label: "Cards" },
	{ key: "table", label: "Table" },
] as const

// Cards or table, for whichever list is on screen. It sits beside the
// page heading rather than over the list: it is a preference about the
// whole page and it is shared by both lists, so a row of its own under
// the title was a band of empty space that said nothing.
export function ViewToggle({ view, onview }: Props): ReactElement {
	return (
		<div className="flex shrink-0 gap-1">
			{views.map(({ key, label }): ReactElement => (
				<Button
					key={key}
					variant={view === key ? "default" : "outline"}
					size="sm"
					aria-pressed={view === key}
					aria-label={label}
					onClick={(): void => {
						onview(key)
					}}
				>
					{label}
				</Button>
			))}
		</div>
	)
}
