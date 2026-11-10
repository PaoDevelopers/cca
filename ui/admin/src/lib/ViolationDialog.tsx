// What a refused write would break, put to the administrator as a list
// rather than a single message. Every negotiable write in the app
// surfaces here: the protocol is uniform, so its presentation is too.
//
// Confirming accepts exactly the violations shown. It is not an
// "override everything" switch — a violation that appears between the
// refusal and the retry is not in the list and still stops the write,
// which is what makes confirming safe under contention.
import { Button } from "@common/components/ui/button"
import {
	Dialog,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
	closeDialog,
} from "@common/components/ui/dialog"
import type { Violation } from "@common/types"
import type { ReactElement } from "react"

interface Props {
	violations: Violation[]
	answer: (accepted: boolean) => void
}

export function ViolationDialog({ violations, answer }: Props): ReactElement {
	// Grouped by student where there is one, so a batch reads as "what
	// is wrong with each person" rather than a flat list of codes.
	const byWho = new Map<string, string[]>()
	for (const v of violations) {
		const who = v.student_id ?? ""
		byWho.set(who, [...(byWho.get(who) ?? []), v.detail])
	}

	return (
		<Dialog
			aria-labelledby="violation-title"
			// The single exit for every path. Escape and the backdrop
			// close it without a returnValue, and no answer is no.
			//
			// Where the focus lands afterwards is the data layer's: it
			// recorded the button that started the write and puts the
			// focus back when the write finishes.
			onClose={(value): void => {
				answer(value === "accept")
			}}
		>
			<DialogHeader>
				<DialogTitle id="violation-title">
					This would break {violations.length} rule
					{violations.length === 1 ? "" : "s"}
				</DialogTitle>
				<DialogDescription>
					Continuing accepts exactly these. Nothing has been saved
					yet.
				</DialogDescription>
			</DialogHeader>

			<ul className="flex flex-col gap-2 text-sm">
				{[...byWho].map(([who, details]) => (
					<li key={who} className="rounded-md border p-3">
						{who !== "" && <strong className="block">{who}</strong>}
						<ul className="list-disc pl-5">
							{details.map((detail, i) => (
								<li key={i}>{detail}</li>
							))}
						</ul>
					</li>
				))}
			</ul>

			<DialogFooter>
				<Button
					type="button"
					variant="outline"
					size="default"
					onClick={(event): void => {
						closeDialog(event.currentTarget, "cancel")
					}}
				>
					Cancel
				</Button>
				<Button
					type="button"
					variant="destructive"
					size="default"
					onClick={(event): void => {
						closeDialog(event.currentTarget, "accept")
					}}
				>
					Continue anyway
				</Button>
			</DialogFooter>
		</Dialog>
	)
}
