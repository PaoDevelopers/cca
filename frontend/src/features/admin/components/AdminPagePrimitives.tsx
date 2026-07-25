import { InboxIcon, SearchIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogMedia,
	AlertDialogTitle,
	AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
	Card,
	CardAction,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "@/components/ui/card"
import {
	Empty,
	EmptyDescription,
	EmptyHeader,
	EmptyMedia,
	EmptyTitle,
} from "@/components/ui/empty"
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "@/components/ui/input-group"
import { Spinner } from "@/components/ui/spinner"

export function PageHeading({
	title,
	description,
	action,
}: {
	title: string
	description: string
	action?: React.ReactNode
}): React.JSX.Element {
	return (
		<div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
			<div className="flex flex-col gap-1">
				<h1 className="font-heading text-xl font-semibold tracking-tight">
					{title}
				</h1>
				<p className="max-w-3xl text-sm text-muted-foreground">
					{description}
				</p>
			</div>
			{action}
		</div>
	)
}

export function SearchBox({
	value,
	onChange,
	placeholder,
}: {
	value: string
	onChange: (value: string) => void
	placeholder: string
}): React.JSX.Element {
	return (
		<InputGroup className="h-11 max-w-md">
			<InputGroupAddon>
				<SearchIcon aria-hidden="true" />
			</InputGroupAddon>
			<InputGroupInput
				type="search"
				name="admin_search"
				value={value}
				onChange={(event) => onChange(event.target.value)}
				placeholder={`${placeholder}…`}
				autoComplete="off"
				spellCheck={false}
				aria-label={placeholder}
			/>
		</InputGroup>
	)
}

export function NoResults({
	title,
	description,
}: {
	title: string
	description: string
}): React.JSX.Element {
	return (
		<Empty>
			<EmptyHeader>
				<EmptyMedia variant="icon">
					<InboxIcon aria-hidden="true" />
				</EmptyMedia>
				<EmptyTitle>{title}</EmptyTitle>
				<EmptyDescription>{description}</EmptyDescription>
			</EmptyHeader>
		</Empty>
	)
}

export function DeleteButton({
	name,
	description,
	onDelete,
	disabled = false,
}: {
	name: string
	description?: string
	onDelete: () => Promise<boolean>
	disabled?: boolean
}): React.JSX.Element {
	const [open, setOpen] = useState(false)
	const [busy, setBusy] = useState(false)

	async function remove(): Promise<void> {
		setBusy(true)
		const deleted = await onDelete()
		setBusy(false)
		if (deleted) setOpen(false)
	}

	return (
		<AlertDialog open={open} onOpenChange={setOpen}>
			<AlertDialogTrigger
				render={
					<Button
						variant="ghost"
						size="icon-sm"
						aria-label={`Delete ${name}`}
						disabled={disabled}
					/>
				}
			>
				<Trash2Icon aria-hidden="true" />
			</AlertDialogTrigger>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogMedia>
						<Trash2Icon aria-hidden="true" />
					</AlertDialogMedia>
					<AlertDialogTitle>Delete {name}?</AlertDialogTitle>
					<AlertDialogDescription>
						{description ??
							"This action cannot be undone. Related records may prevent deletion."}
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel disabled={busy}>
						Cancel
					</AlertDialogCancel>
					<AlertDialogAction
						variant="destructive"
						disabled={busy}
						onClick={() => void remove()}
					>
						{busy ? (
							<Spinner data-icon="inline-start" />
						) : (
							<Trash2Icon data-icon="inline-start" />
						)}
						Delete
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	)
}

export function StatCard({
	icon: Icon,
	label,
	value,
	description,
}: {
	icon: React.ComponentType<React.SVGProps<SVGSVGElement>>
	label: string
	value: number
	description: string
}): React.JSX.Element {
	return (
		<Card>
			<CardHeader>
				<CardTitle>{label}</CardTitle>
				<CardDescription>{description}</CardDescription>
				<CardAction>
					<Badge variant="secondary">
						<Icon aria-hidden="true" />
						<span className="sr-only">{label}</span>
					</Badge>
				</CardAction>
			</CardHeader>
			<CardContent>
				<p className="font-heading text-3xl font-semibold tabular-nums">
					{value}
				</p>
			</CardContent>
		</Card>
	)
}
