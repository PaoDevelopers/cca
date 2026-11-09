// Bulk operations for one data section, behind one menu: they are
// occasional, and the page is for the rows. Import and the full reset
// open dialogs; the reset is gated behind typing an exact phrase.
import { clearSection, importSpreadsheet } from "@common/adminApi"
import { Button } from "@common/components/ui/button"
import {
	Dialog,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
	closeDialog,
} from "@common/components/ui/dialog"
import {
	DropdownMenu,
	DropdownMenuItem,
	DropdownMenuLink,
} from "@common/components/ui/dropdown-menu"
import { Input, Label } from "@common/components/ui/input"
import { toast } from "@common/components/ui/toast"
import { useState, type ReactElement, type ReactNode } from "react"
import { TrashIcon } from "./ConfirmDelete"

interface Props {
	section: string
	exportHref?: string | undefined
	importAction?: string | undefined
	// Takes the upload over from the plain import, for a section whose
	// import is a conversation rather than one request.
	onimport?: ((file: File) => void) | undefined
	busy: boolean
	run: (action: () => Promise<void>) => Promise<boolean>
	// A section's own menu items, above the ones every section gets.
	extra?: ReactNode
}

const confirmation = "I confirm that I wish to destroy all data here."

export function DataTools({
	section,
	exportHref,
	importAction,
	onimport,
	busy,
	run,
	extra,
}: Props): ReactElement {
	const [dialog, setDialog] = useState<"import" | "reset" | null>(null)
	const [file, setFile] = useState<File | null>(null)
	const [phrase, setPhrase] = useState("")

	const close = (): void => {
		setDialog(null)
		setFile(null)
		setPhrase("")
	}

	return (
		<>
			<DropdownMenu
				trigger={(props): ReactElement => (
					<Button
						type="button"
						variant="outline"
						size="sm"
						{...props}
					>
						Data tools
					</Button>
				)}
			>
				{extra}
				{exportHref !== undefined && (
					<DropdownMenuLink href={exportHref}>
						Export {section} as CSV
					</DropdownMenuLink>
				)}
				{importAction !== undefined && (
					<>
						{/*
							An import with no example is how a wrong file
							gets made: the columns are not guessable, and
							the identifier forms are not either. Named by
							section rather than passed in, so a section
							that imports always has one; that the files'
							headers still match the importers is held by
							TestTheShippedExamplesHaveTheHeadersTheImportersWant.
						*/}
						<DropdownMenuLink
							href={`/admin/static/${section}_example.csv`}
							download
						>
							Download {section} CSV template
						</DropdownMenuLink>
						<DropdownMenuItem
							onClick={(): void => {
								setDialog("import")
							}}
						>
							Import {section}…
						</DropdownMenuItem>
					</>
				)}
				<DropdownMenuItem
					className="text-destructive"
					onClick={(): void => {
						setDialog("reset")
					}}
				>
					<TrashIcon />
					Delete all {section}…
				</DropdownMenuItem>
			</DropdownMenu>

			{dialog === "import" && importAction !== undefined && (
				<Dialog onClose={close}>
					<form
						onSubmit={(event): void => {
							event.preventDefault()
							if (file === null) {
								return
							}
							closeDialog(event.currentTarget, "import")
							if (onimport !== undefined) {
								onimport(file)
								return
							}
							// No override: an import accepts nothing. A row
							// that would break a rule is reported so somebody
							// decides on it deliberately, rather than being
							// waved through by the act of uploading a file.
							void run((): Promise<void> =>
								importSpreadsheet(importAction, file),
							).then((ok): void => {
								if (ok) {
									toast(
										`Imported ${file.name} into ${section}.`,
									)
								}
							})
						}}
					>
						<DialogHeader>
							<DialogTitle>Import {section}</DialogTitle>
							<DialogDescription>
								Upload the CSV template format directly, or put
								the same columns in the first row of one visible
								worksheet in an XLSX file. Paste formulas as
								values first.
							</DialogDescription>
						</DialogHeader>
						<Label className="flex-col items-start">
							Import CSV or XLSX
							<Input
								type="file"
								accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
								required
								onChange={(event): void => {
									setFile(
										event.currentTarget.files?.item(0) ??
											null,
									)
								}}
							/>
						</Label>
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
								size="default"
								disabled={busy || file === null}
							>
								Import into {section}
							</Button>
						</DialogFooter>
					</form>
				</Dialog>
			)}

			{dialog === "reset" && (
				<Dialog role="alertdialog" onClose={close}>
					<form
						onSubmit={(event): void => {
							event.preventDefault()
							if (phrase !== confirmation) {
								return
							}
							closeDialog(event.currentTarget, "reset")
							void run((): Promise<void> => clearSection(section))
						}}
					>
						<DialogHeader>
							<DialogTitle>Delete all {section}?</DialogTitle>
							<DialogDescription>
								Every row in this section goes, and cannot be
								restored.
							</DialogDescription>
						</DialogHeader>
						<Label className="flex-col items-start leading-normal">
							Type “{confirmation}” to enable the reset
							<Input
								type="text"
								value={phrase}
								onChange={(event): void => {
									setPhrase(event.currentTarget.value)
								}}
							/>
						</Label>
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
								variant="destructive"
								size="default"
								disabled={busy || phrase !== confirmation}
								aria-label={`Delete all ${section}`}
							>
								Delete all {section}
							</Button>
						</DialogFooter>
					</form>
				</Dialog>
			)}
		</>
	)
}
