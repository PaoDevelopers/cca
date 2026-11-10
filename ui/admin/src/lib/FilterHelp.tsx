// A popover documenting the active filter syntax and the fields
// available on the current page.
import type { FilterMode } from "@common/cel"
import { Button } from "@common/components/ui/button"
import { DropdownMenu } from "@common/components/ui/dropdown-menu"
import type { ReactElement } from "react"

interface Props {
	fields: Record<string, string>
	mode: FilterMode
	// A worked expression, shown for CEL only.
	example?: string | undefined
}

export function FilterHelp({ fields, mode, example }: Props): ReactElement {
	return (
		<DropdownMenu
			className="max-w-md p-3 text-sm"
			trigger={(props): ReactElement => (
				<Button
					type="button"
					variant="outline"
					size="icon"
					aria-label="Filter syntax help"
					{...props}
				>
					?
				</Button>
			)}
		>
			{mode === "cel" ? (
				<>
					<p className="mb-2">
						A{" "}
						<a className="underline" href="https://cel.dev/">
							CEL
						</a>{" "}
						expression evaluated per row, returning true or false.
						Strings need quotes and match exactly;{" "}
						<code>.contains()</code>, <code>.lowerAscii()</code> and{" "}
						<code>.matches()</code> are available, and{" "}
						<code>in</code> tests list membership.
					</p>
					{example !== undefined && (
						<p className="mb-2">
							e.g. <code>{example}</code>
						</p>
					)}
				</>
			) : (
				<p className="mb-2">
					Space-separated terms; rows must match all of them. A bare
					term matches anywhere. <code>field:value</code> matches one
					field, and <code>field:a,b</code> matches either value.
					Matching is case-insensitive; fields marked exact match
					whole values only, the rest match substrings.
				</p>
			)}
			<dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
				{Object.entries(fields).map(([name, description]) => (
					<div key={name} className="contents">
						<dt>
							<code>{name}</code>
						</dt>
						<dd className="text-muted-foreground">{description}</dd>
					</div>
				))}
			</dl>
		</DropdownMenu>
	)
}
