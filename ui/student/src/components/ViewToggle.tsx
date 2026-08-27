import type { ReactElement } from "react"
import { LayoutGrid, Rows3 } from "lucide-react"
import { Button } from "@/components/ui/button"

interface Props {
	view: "cards" | "table"
	onview: (view: "cards" | "table") => void
}

const views = [
	{ key: "cards", label: "Cards", Icon: LayoutGrid },
	{ key: "table", label: "Table", Icon: Rows3 },
] as const

// Cards or table, for whichever list is on screen. It sits beside the
// page heading rather than over the list: it is a preference about the
// whole page and it is shared by both lists, so a row of its own under
// the title was a band of empty space that said nothing.
export function ViewToggle({ view, onview }: Props): ReactElement {
	return (
		<div className="flex shrink-0 gap-1">
			{views.map(({ key, label, Icon }): ReactElement => (
				<Button
					key={key}
					variant={view === key ? "default" : "outline"}
					size="icon"
					aria-pressed={view === key}
					aria-label={label}
					onClick={(): void => {
						onview(key)
					}}
				>
					<Icon aria-hidden="true" />
				</Button>
			))}
		</div>
	)
}
