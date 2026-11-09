import * as React from "react"

import { cn } from "@common/lib/utils"

// The default size is wrapped in :where(), which has no specificity, so
// a width or height passed in className wins without a class merger.
const field =
	"[:where(&)]:w-full min-w-0 rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs transition-[color,box-shadow] outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:bg-input/30"

function Input({ className, ...props }: React.ComponentProps<"input">) {
	return (
		<input
			data-slot="input"
			className={cn(
				field,
				"[:where(&)]:h-9 file:mr-2 file:border-0 file:bg-transparent file:text-sm file:font-medium",
				className,
			)}
			{...props}
		/>
	)
}

function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
	return (
		<textarea
			data-slot="textarea"
			className={cn(field, "min-h-16 py-2", className)}
			{...props}
		/>
	)
}

// A native <select>: it brings the keyboard and screen-reader behaviour
// for free, which a hand-rolled listbox would have to re-earn.
function NativeSelect({ className, ...props }: React.ComponentProps<"select">) {
	return (
		<select
			data-slot="native-select"
			className={cn(
				field,
				"[:where(&)]:h-9 [:where(&)]:w-auto pr-8",
				className,
			)}
			{...props}
		/>
	)
}

function Label({ className, ...props }: React.ComponentProps<"label">) {
	return (
		<label
			data-slot="label"
			className={cn(
				"flex items-center gap-2 text-sm leading-none font-medium select-none has-disabled:opacity-50",
				className,
			)}
			{...props}
		/>
	)
}

function Checkbox({ className, ...props }: React.ComponentProps<"input">) {
	return (
		<input
			type="checkbox"
			data-slot="checkbox"
			className={cn("size-4 shrink-0 accent-primary", className)}
			{...props}
		/>
	)
}

export { Input, Textarea, NativeSelect, Label, Checkbox }
