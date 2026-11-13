// Joins class names; it does not merge them. When a caller's class
// sets the same property as one the component already has (max-w-6xl
// against a dialog's max-w-lg), the one later in Tailwind's generated
// stylesheet wins, whichever order they were written in. Most such
// overrides happen to win today; a dialog's width did not. Mark an
// override that must win with the important modifier: max-w-6xl!.
export function cn(
	...classes: Array<string | false | null | undefined>
): string {
	return classes.filter(Boolean).join(" ")
}
