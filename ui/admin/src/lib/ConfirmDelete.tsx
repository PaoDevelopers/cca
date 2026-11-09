// A destructive action behind a confirmation dialog that names what is
// about to go.
//
// The open state is per instance, so a refetch that reorders the rows
// cannot move the question from the row somebody looked at to another.
import { Button } from "@common/components/ui/button"
import {
	Dialog,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
	closeDialog,
} from "@common/components/ui/dialog"
import { useState, type ReactElement } from "react"

interface Props {
	// The action verb, e.g. "Delete"; also builds accessible names.
	action: string
	// Reads after the verb: "the enrollment of BB for 1".
	target: string
	disabled: boolean
	onconfirm: () => void
	// What else goes with it, if anything.
	detail?: string | undefined
}

// A drawn trash can, the mark for every delete in the panel.
export function TrashIcon(): ReactElement {
	return (
		<svg
			aria-hidden="true"
			viewBox="0 0 16 16"
			className="size-4"
			fill="none"
			stroke="currentColor"
			strokeWidth="1.5"
			strokeLinecap="round"
			strokeLinejoin="round"
		>
			<path d="M2.5 4.5h11M6.5 4.5V3a1 1 0 0 1 1-1h1a1 1 0 0 1 1 1v1.5M4 4.5l.7 8.6a1 1 0 0 0 1 .9h4.6a1 1 0 0 0 1-.9l.7-8.6M6.75 7v4.5M9.25 7v4.5" />
		</svg>
	)
}

export function ConfirmDelete({
	action,
	target,
	disabled,
	onconfirm,
	detail,
}: Props): ReactElement {
	const [open, setOpen] = useState(false)

	return (
		<>
			<Button
				type="button"
				variant="ghost"
				size="icon-sm"
				className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
				disabled={disabled}
				aria-label={`${action} ${target}`}
				// The icon alone says "delete"; the tooltip says what.
				title={`${action} ${target}`}
				onClick={(): void => {
					setOpen(true)
				}}
			>
				<TrashIcon />
			</Button>
			{open && (
				<Dialog
					role="alertdialog"
					onClose={(value): void => {
						setOpen(false)
						if (value === "confirm") {
							onconfirm()
						}
					}}
				>
					<DialogHeader>
						<DialogTitle>
							{action} {target}?
						</DialogTitle>
						<DialogDescription>
							{detail ?? "This cannot be undone."}
						</DialogDescription>
					</DialogHeader>
					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							size="default"
							aria-label={`Cancel ${action.toLowerCase()} of ${target}`}
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
							aria-label={`Confirm ${action.toLowerCase()} of ${target}`}
							onClick={(event): void => {
								closeDialog(event.currentTarget, "confirm")
							}}
						>
							{action}
						</Button>
					</DialogFooter>
				</Dialog>
			)}
		</>
	)
}
