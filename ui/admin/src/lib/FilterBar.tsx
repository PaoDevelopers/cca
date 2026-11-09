// The filter control shared by the admin lists. CEL gets a textarea;
// its expressions run off the end of a one-line input.
import type { FilterMode } from "@common/cel"
import { Button } from "@common/components/ui/button"
import { Input, NativeSelect, Textarea } from "@common/components/ui/input"
import type { ReactElement } from "react"
import { FilterHelp } from "./FilterHelp"

interface Props {
	query: string
	onQuery: (query: string) => void
	mode: FilterMode
	onMode: (mode: FilterMode) => void
	// Field descriptions for the simple-mode help popover.
	fields: Record<string, string>
	// Dotted paths available to a CEL expression.
	celFields: Record<string, string>
	// A worked CEL expression for the help popover.
	celExample: string
	// Parse or evaluation error from the current CEL expression.
	error?: string | null | undefined
	// The "N of M shown" line, beside the box it describes.
	summary?: string | undefined
}

export function FilterBar({
	query,
	onQuery,
	mode,
	onMode,
	fields,
	celFields,
	celExample,
	error = null,
	summary,
}: Props): ReactElement {
	return (
		<div className="flex flex-col gap-2">
			<div className="flex flex-wrap items-center gap-2">
				{/*
					Named by aria-label rather than a wrapping <label>: React
					keeps a controlled textarea's text content equal to its
					value, so a wrapping label's name would grow with every
					expression typed into it.
				*/}
				<div className="flex min-w-0 flex-1 items-center gap-2">
					{mode === "cel" ? (
						<Textarea
							rows={2}
							spellCheck={false}
							aria-label="Filter"
							placeholder="CEL expression"
							className="min-w-64 font-mono"
							value={query}
							onChange={(event): void => {
								onQuery(event.currentTarget.value)
							}}
						/>
					) : (
						<Input
							type="search"
							aria-label="Filter"
							placeholder="Filter…"
							value={query}
							onChange={(event): void => {
								onQuery(event.currentTarget.value)
							}}
						/>
					)}
				</div>
				<Button
					type="button"
					variant="outline"
					size="default"
					aria-label="Clear the filter"
					disabled={query === ""}
					onClick={(): void => {
						onQuery("")
					}}
				>
					Clear
				</Button>
				<label>
					<span className="sr-only">Filter mode</span>
					<NativeSelect
						value={mode}
						onChange={(event): void => {
							onMode(event.currentTarget.value as FilterMode)
						}}
					>
						<option value="simple">Simple</option>
						<option value="cel">CEL</option>
					</NativeSelect>
				</label>
				<FilterHelp
					mode={mode}
					fields={mode === "cel" ? celFields : fields}
					example={mode === "cel" ? celExample : undefined}
				/>
				{summary !== undefined && (
					<span className="ml-auto text-sm text-muted-foreground">
						{summary}
					</span>
				)}
			</div>
			{error !== null && (
				<p
					role="alert"
					className="rounded-md border border-destructive bg-destructive/10 px-3 py-2 text-sm"
				>
					{error}
				</p>
			)}
		</div>
	)
}
