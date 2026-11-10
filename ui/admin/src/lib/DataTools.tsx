// Bulk operations for one data section, in a panel that expands in
// place above the rows: they are occasional, and stay folded away
// until asked for. The full reset is gated behind typing an exact
// confirmation phrase.
import { clearSection, importSpreadsheet } from "@common/adminApi"
import { Button } from "@common/components/ui/button"
import { Input, Label } from "@common/components/ui/input"
import { toast } from "@common/components/ui/toast"
import { useId, useState, type ReactElement, type ReactNode } from "react"
import { TrashIcon, UploadIcon } from "./icons"

interface Props {
	section: string
	exportHref?: string | undefined
	importAction?: string | undefined
	// Takes the upload over from the plain import, for a section whose
	// import is a conversation rather than one request.
	onimport?: ((file: File) => void) | undefined
	busy: boolean
	run: (action: () => Promise<void>) => Promise<boolean>
	// A section's own tools, beside the ones every section gets.
	extra?: ReactNode
}

const confirmation = "I confirm that I wish to destroy all data here."

const linkClass =
	"text-sm font-medium underline underline-offset-4 hover:text-primary"

export function DataTools({
	section,
	exportHref,
	importAction,
	onimport,
	busy,
	run,
	extra,
}: Props): ReactElement {
	const [file, setFile] = useState<File | null>(null)
	const [phrase, setPhrase] = useState("")
	const uid = useId()

	return (
		<details className="data-tools group rounded-lg border">
			<summary className="flex cursor-pointer list-none items-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium select-none hover:bg-accent/50 [&::-webkit-details-marker]:hidden">
				<svg
					aria-hidden="true"
					viewBox="0 0 16 16"
					className="size-4 text-muted-foreground transition-transform group-open:rotate-90"
					fill="none"
					stroke="currentColor"
					strokeWidth="1.5"
					strokeLinecap="round"
					strokeLinejoin="round"
				>
					<path d="m6 4 4 4-4 4" />
				</svg>
				Data tools
			</summary>

			<div className="flex flex-col gap-5 border-t px-4 py-4">
				{(extra !== undefined ||
					exportHref !== undefined ||
					importAction !== undefined) && (
					<div className="flex flex-wrap items-center gap-x-5 gap-y-2">
						{extra}
						{exportHref !== undefined && (
							<a className={linkClass} href={exportHref}>
								Export {section} as CSV
							</a>
						)}
						{importAction !== undefined && (
							// An import with no example is how a wrong file
							// gets made: the columns are not guessable, and
							// the identifier forms are not either. Named by
							// section rather than passed in, so a section
							// that imports always has one; that the files'
							// headers still match the importers is held by
							// TestTheShippedExamplesHaveTheHeadersTheImportersWant.
							<a
								className={linkClass}
								href={`/admin/static/${section}_example.csv`}
								download
							>
								Download {section} CSV template
							</a>
						)}
					</div>
				)}

				{importAction !== undefined && (
					<form
						className="flex flex-col gap-2"
						onSubmit={(event): void => {
							event.preventDefault()
							if (file === null) {
								return
							}
							const form = event.currentTarget
							const done = (): void => {
								form.reset()
								setFile(null)
							}
							if (onimport !== undefined) {
								onimport(file)
								done()
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
									done()
									toast(
										`Imported ${file.name} into ${section}.`,
									)
								}
							})
						}}
					>
						<p className="max-w-3xl text-sm text-muted-foreground">
							Upload that CSV format directly, or put the same
							columns in the first row of one visible worksheet in
							an XLSX file. Paste formulas as values first.
						</p>
						<div className="flex flex-wrap items-end gap-2">
							<Label className="flex-col items-start">
								Import CSV or XLSX
								<Input
									type="file"
									className="w-80 max-w-full"
									accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
									required
									onChange={(event): void => {
										setFile(
											event.currentTarget.files?.item(
												0,
											) ?? null,
										)
									}}
								/>
							</Label>
							<Button
								size="icon"
								disabled={busy || file === null}
								aria-label={`Import into ${section}`}
								tooltip={`Import into ${section}`}
							>
								<UploadIcon />
							</Button>
						</div>
					</form>
				)}

				<form
					className="flex flex-col gap-2 border-t pt-4"
					onSubmit={(event): void => {
						event.preventDefault()
						if (phrase !== confirmation) {
							return
						}
						void run((): Promise<void> =>
							clearSection(section),
						).then((ok): void => {
							if (ok) {
								setPhrase("")
							}
						})
					}}
				>
					<Label htmlFor={`${uid}-phrase`} className="leading-normal">
						Type “{confirmation}” to enable the reset
					</Label>
					<div className="flex flex-wrap gap-2">
						<Input
							id={`${uid}-phrase`}
							type="text"
							className="w-96 max-w-full"
							value={phrase}
							onChange={(event): void => {
								setPhrase(event.currentTarget.value)
							}}
						/>
						<Button
							variant="destructive"
							size="default"
							disabled={busy || phrase !== confirmation}
							aria-label={`Delete all ${section}`}
						>
							<TrashIcon />
							Delete all {section}
						</Button>
					</div>
				</form>
			</div>
		</details>
	)
}
