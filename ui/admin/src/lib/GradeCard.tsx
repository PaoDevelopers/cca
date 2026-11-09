import {
	closeWindowNow,
	deleteGrade,
	openWindowNow,
	setGradeBudget,
	setGradeRequirements,
	setGradeWindow,
	updateGrade,
} from "@common/adminApi"
import { Button } from "@common/components/ui/button"
import { DropdownMenu } from "@common/components/ui/dropdown-menu"
import { Checkbox, Input, Label } from "@common/components/ui/input"
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@common/components/ui/table"
import type { Category, Grade } from "@common/types"
import {
	useEffect,
	useId,
	useRef,
	useState,
	type ReactElement,
	type ReactNode,
} from "react"
import { ConfirmDelete } from "./ConfirmDelete"
import { TrashIcon } from "./icons"
import { formatInstant, fromLocalInput, toLocalInput } from "./datetime"

interface Props {
	grade: Grade
	categories: Category[]
	busy: boolean
	run: (action: () => Promise<void>) => Promise<boolean>
	// For the budget cap, which is the one field here that can break an
	// existing enrollment.
	runAccepting: (
		action: (accept: string[]) => Promise<void>,
	) => Promise<boolean>
	move: (direction: "up" | "down") => void
}

// A drawn chevron, one stroke weight with the rest of the card's marks.
function Chevron({ direction }: { direction: "up" | "down" }): ReactElement {
	return (
		<svg
			aria-hidden="true"
			viewBox="0 0 16 16"
			className="size-4"
			fill="none"
			stroke="currentColor"
			strokeWidth="1.75"
			strokeLinecap="round"
			strokeLinejoin="round"
		>
			<path d={direction === "up" ? "M4 10l4-4 4 4" : "M4 6l4 4 4-4"} />
		</svg>
	)
}

type SaveState = "saving" | "saved" | null

// What an autosaving field says about its last write. "Saved" fades
// after a moment; a refusal says nothing here, because the error
// popover already does.
function useSaveState(): [SaveState, (write: Promise<boolean>) => void] {
	const [state, setState] = useState<SaveState>(null)
	const timer = useRef<ReturnType<typeof setTimeout>>(undefined)

	useEffect((): (() => void) => (): void => clearTimeout(timer.current), [])

	const track = (write: Promise<boolean>): void => {
		clearTimeout(timer.current)
		setState("saving")
		void write.then((applied): void => {
			setState(applied ? "saved" : null)
			if (applied) {
				timer.current = setTimeout((): void => {
					setState(null)
				}, 2000)
			}
		})
	}

	return [state, track]
}

const saveText = { saving: "Saving…", saved: "Saved ✓" } as const

function SaveStatus({ state }: { state: SaveState }): ReactElement {
	return (
		<span
			aria-live="polite"
			className="min-w-14 text-xs text-muted-foreground"
		>
			{state === null ? "" : saveText[state]}
		</span>
	)
}

function Help({
	label,
	children,
}: {
	label: string
	children: ReactNode
}): ReactElement {
	return (
		<DropdownMenu
			className="max-w-sm p-3 text-sm"
			trigger={(props): ReactElement => (
				<Button
					type="button"
					variant="ghost"
					size="icon-sm"
					aria-label={label}
					{...props}
				>
					?
				</Button>
			)}
		>
			{children}
		</DropdownMenu>
	)
}

// The requirement set is one form and one save, so it is edited as a
// whole here and sent as a whole. Nothing is written until Save. The
// count is held as text: an empty box is not zero, and zero is a
// meaningful requirement here — permanently met — so an empty box that
// became zero would quietly tell every student they had satisfied
// something they had not.
interface RequirementDraft {
	count: string
	category_ids: string[]
}

function draftsOf(of: Grade): RequirementDraft[] {
	return of.requirements.map((r) => ({
		count: String(r.min_period_count),
		category_ids: [...r.category_ids],
	}))
}

interface Requirement {
	min_period_count: number
	category_ids: string[]
}

// What the drafts mean, or the first reason they mean nothing.
function readRequirements(
	drafts: RequirementDraft[],
): { ok: true; value: Requirement[] } | { ok: false; why: string } {
	const value: Requirement[] = []
	for (const [i, draft] of drafts.entries()) {
		if (draft.category_ids.length === 0) {
			return {
				ok: false,
				why: `Requirement ${String(i + 1)} names no category.`,
			}
		}
		const count = Number.parseInt(draft.count.trim(), 10)
		if (!Number.isInteger(count) || count < 0) {
			return {
				ok: false,
				why: `Requirement ${String(i + 1)} needs a whole number of periods.`,
			}
		}
		value.push({
			min_period_count: count,
			category_ids: draft.category_ids,
		})
	}
	return { ok: true, value }
}

// Spell out the nullable-bound semantics. A dash beside an empty box
// made the two very different kinds of absence look the same: no
// opening keeps the window closed, while no closing lets an open window
// continue indefinitely.
function windowSummary(grade: Grade): string {
	if (grade.is_open) {
		return grade.closes_at === null
			? `Open since ${formatInstant(grade.opens_at)}; no automatic closing time.`
			: `Open since ${formatInstant(grade.opens_at)}; closes ${formatInstant(grade.closes_at)}.`
	}
	if (grade.opens_at === null) {
		return grade.closes_at === null
			? "Closed; no opening or closing time is set."
			: `Closed; no opening time is set. The saved closing time is ${formatInstant(grade.closes_at)}.`
	}
	return grade.closes_at === null
		? `Closed; opens ${formatInstant(grade.opens_at)} and then stays open.`
		: `Closed; scheduled ${formatInstant(grade.opens_at)} to ${formatInstant(grade.closes_at)}.`
}

export function GradeCard({
	grade,
	categories,
	busy,
	run,
	runAccepting,
	move,
}: Props): ReactElement {
	// The card is labelled by the grade id, which is not the article's
	// first child.
	const uid = useId()

	// Every box below follows the server while nobody is typing in it,
	// and belongs to whoever is typing once they start: re-deriving it
	// from the prop would discard a half-typed value every time the list
	// is refetched, which happens after any grade write anywhere —
	// including another card on this page. Seeded once and never re-read
	// instead, a box went on showing a value somebody else had changed
	// for as long as the page stayed open. So each one shows its draft
	// while dirty and the server's value otherwise, until its write lands.
	const [nameDraft, setNameDraft] = useState<string | null>(null)
	const [nameState, trackName] = useSaveState()

	// The window is two instants, and each box owns one of them.
	//
	// Both used to be sent on every change, which quietly reverted the
	// other administrator's work: a card that had been open while
	// somebody else moved the opening time still held the old one, and
	// editing the closing time wrote it back. Nobody saw it happen — the
	// write succeeded, and the value it restored was a value that had
	// genuinely been there.
	//
	// So each write sends only the boxes this administrator actually
	// edited, and leaves the other key out of the request entirely.
	// Omission is what makes it safe: an absent bound is one the database
	// does not touch. Sending "the server's current value for the other"
	// would be the bug again, one read earlier.
	const [opensDraft, setOpensDraft] = useState<string | null>(null)
	const [closesDraft, setClosesDraft] = useState<string | null>(null)
	const [windowState, trackWindow] = useSaveState()

	// The two numbers are held as text: an empty budget is "no cap",
	// which is a real setting and not the same as zero.
	const budgetValue =
		grade.max_budgeted_periods === null
			? ""
			: String(grade.max_budgeted_periods)
	const [budgetDraft, setBudgetDraft] = useState<string | null>(null)
	const [budgetState, trackBudget] = useSaveState()
	const [minDraft, setMinDraft] = useState<string | null>(null)
	const [minState, trackMin] = useSaveState()

	// The requirement set is written declaratively — the whole list
	// replaces the whole list — so a card built from a stale copy of it
	// deletes whatever has been added since. This card lives for the
	// whole session (GradesPage keys it by grade id), so "since" can be
	// days: administrator A opens the page on Monday, B adds a
	// requirement on Tuesday, A adjusts a period count on Friday and
	// writes back Monday's list, and B's requirement is gone with no
	// error and nothing to see. There is no half of this list to omit,
	// as the window bounds do.
	//
	// So the same rule instead. Untouched, the drafts follow the server.
	// Touched, they are the administrator's and are left alone — and if
	// the server's list moves underneath them while they are touched,
	// the write is refused rather than allowed to silently undo it.
	const requirementsOnServer = JSON.stringify(draftsOf(grade))
	const [touched, setTouched] = useState<{
		drafts: RequirementDraft[]
		basis: string
	} | null>(null)
	const requirementDrafts = touched?.drafts ?? draftsOf(grade)
	const requirementsMovedElsewhere =
		touched !== null && requirementsOnServer !== touched.basis
	const [requirementError, setRequirementError] = useState<string | null>(
		null,
	)

	function editRequirements(
		edit: (drafts: RequirementDraft[]) => RequirementDraft[],
	): void {
		setTouched(
			touched === null
				? { drafts: edit(draftsOf(grade)), basis: requirementsOnServer }
				: { ...touched, drafts: edit(touched.drafts) },
		)
	}

	function saveName(): void {
		if (nameDraft === null) {
			return
		}
		const name = nameDraft.trim()
		if (name === "" || name === grade.name) {
			setNameDraft(null)
			return
		}
		// A refused name stays in the box, to be corrected rather than
		// retyped.
		const write = run(async (): Promise<void> => {
			await updateGrade(grade.id, name, grade.min_distinct_categories)
			setNameDraft(null)
		})
		trackName(write)
	}

	// Sends only the boxes this administrator touched. A bound left out
	// is left alone, so the value the other box was built with — which
	// may be older than what the grade now has — never travels.
	function saveWindow(): void {
		const bounds: { opens_at?: string | null; closes_at?: string | null } =
			{}
		if (opensDraft !== null) {
			bounds.opens_at = fromLocalInput(opensDraft)
		}
		if (closesDraft !== null) {
			bounds.closes_at = fromLocalInput(closesDraft)
		}
		if (bounds.opens_at === undefined && bounds.closes_at === undefined) {
			return
		}
		// Dirty until the write lands, so a half-set date is not yanked
		// away mid-edit by a refetch some other card triggered.
		const write = run(async (): Promise<void> => {
			await setGradeWindow(grade.id, bounds)
			setOpensDraft(null)
			setClosesDraft(null)
		})
		trackWindow(write)
	}

	function saveBudget(): void {
		if (budgetDraft === null || budgetDraft === budgetValue) {
			setBudgetDraft(null)
			return
		}
		const raw = budgetDraft.trim()
		const cap = raw === "" ? null : Number.parseInt(raw, 10)
		if (cap !== null && (Number.isNaN(cap) || cap < 0)) {
			setBudgetDraft(null)
			return
		}
		// Lowering the cap can put students already enrolled over it, so
		// this is the one setting here that can come back asking to be
		// confirmed — and declining, or a refusal, must put the box back:
		// the value it holds otherwise is one the grade does not have.
		const write = runAccepting((accept): Promise<void> =>
			setGradeBudget(grade.id, cap, accept),
		)
		trackBudget(write)
		void write.then((): void => {
			setBudgetDraft(null)
		})
	}

	function saveMin(): void {
		if (
			minDraft === null ||
			minDraft === String(grade.min_distinct_categories)
		) {
			setMinDraft(null)
			return
		}
		const min = Number.parseInt(minDraft.trim(), 10)
		if (Number.isNaN(min) || min < 0) {
			setMinDraft(null)
			return
		}
		const write = run((): Promise<void> =>
			updateGrade(grade.id, grade.name, min),
		)
		trackMin(write)
		void write.then((): void => {
			setMinDraft(null)
		})
	}

	// The manual levers, which the scheduled bounds subsume.
	//
	// They do not go through the boxes. A draft is an instant this
	// administrator chose and is theirs until it is written; "now" is
	// not chosen, it is read, and the reading that matters belongs to
	// the database — the same clock v_grades.is_open is evaluated
	// against. Routing these through the drafts meant a third clock
	// wrote the value, rounded to the minute, so pressing both inside
	// one minute sent closes_at = opens_at and was refused by the CHECK.
	//
	// So they send nothing and take nothing back. The refetch that
	// follows the write brings the new bounds in, which is also why
	// neither touches a draft: there is no half-finished edit here to
	// protect.
	const windowSection = (
		// The bounds and what they mean stay together; acting now is a
		// separate decision, so it sits apart, below them.
		<fieldset className="@container flex flex-col gap-4">
			<legend className="mb-3 text-sm font-semibold">
				Enrollment window
			</legend>
			<div className="flex flex-col gap-1.5">
				<div className="grid gap-3 @sm:grid-cols-2">
					<Label className="flex-col items-start">
						Opens
						<Input
							type="datetime-local"
							aria-label={`Window opens for ${grade.id}`}
							value={opensDraft ?? toLocalInput(grade.opens_at)}
							onChange={(event): void => {
								setOpensDraft(event.currentTarget.value)
							}}
							onBlur={saveWindow}
						/>
					</Label>
					<Label className="flex-col items-start">
						Closes
						<Input
							type="datetime-local"
							aria-label={`Window closes for ${grade.id}`}
							value={closesDraft ?? toLocalInput(grade.closes_at)}
							onChange={(event): void => {
								setClosesDraft(event.currentTarget.value)
							}}
							onBlur={saveWindow}
						/>
					</Label>
				</div>
				<div className="flex items-baseline justify-between gap-3">
					<p className="text-xs text-muted-foreground">
						Empty opening = closed. Empty closing = no automatic
						close.
					</p>
					<SaveStatus state={windowState} />
				</div>
			</div>
			<div className="flex items-center gap-2">
				<Button
					type="button"
					size="sm"
					disabled={busy || grade.is_open}
					onClick={(): void => {
						void run((): Promise<void> => openWindowNow(grade.id))
					}}
				>
					Open now
				</Button>
				<Button
					type="button"
					variant="outline"
					size="sm"
					disabled={busy || !grade.is_open}
					onClick={(): void => {
						void run((): Promise<void> => closeWindowNow(grade.id))
					}}
				>
					Close now
				</Button>
			</div>
		</fieldset>
	)

	const limitsSection = (
		<fieldset className="flex flex-wrap items-center gap-2">
			<legend className="mb-3 text-sm font-semibold">Limits</legend>
			<span className="text-sm font-medium">Period budget</span>
			<Help label="Explain the period budget">
				The number of periods a student in this grade may occupy with
				budgeted enrollments. A course meeting twice a week counts 2.
				Leave empty for no cap.
			</Help>
			<Input
				type="number"
				min="0"
				placeholder="none"
				className="w-24"
				aria-label={`Period budget for ${grade.id}`}
				value={budgetDraft ?? budgetValue}
				onChange={(event): void => {
					setBudgetDraft(event.currentTarget.value)
				}}
				onBlur={saveBudget}
			/>
			<SaveStatus state={budgetState} />
		</fieldset>
	)

	// Requirements are advisory and are edited as a set: rows are added
	// and removed here, and one Save replaces the lot. Nothing is
	// written until then, so a half-built requirement never reaches the
	// server.
	const requirementsSection = (
		<fieldset className="flex flex-col gap-3">
			<legend className="mb-3 text-sm font-semibold">Requirements</legend>
			<div className="flex flex-wrap items-center gap-2">
				<span className="text-sm font-medium">Distinct categories</span>
				<Help label="Explain the category minimum">
					Enrollments in scheduled courses must span at least this
					many different categories; 0 disables the requirement.
					Advisory: it is reported to the student, never enforced.
				</Help>
				<Input
					type="number"
					min="0"
					className="w-24"
					aria-label={`Distinct categories for ${grade.id}`}
					value={minDraft ?? String(grade.min_distinct_categories)}
					onChange={(event): void => {
						setMinDraft(event.currentTarget.value)
					}}
					onBlur={saveMin}
				/>
				<SaveStatus state={minState} />
			</div>
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead scope="col">Categories</TableHead>
						<TableHead scope="col">Periods required</TableHead>
						<TableHead scope="col">
							<span className="sr-only">Actions</span>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{requirementDrafts.map((requirement, index) => (
						<TableRow key={index}>
							<TableCell className="flex flex-wrap gap-x-3 gap-y-1">
								{categories.map((category) => (
									<Label
										key={category.id}
										className="font-normal"
									>
										<Checkbox
											value={category.id}
											checked={requirement.category_ids.includes(
												category.id,
											)}
											onChange={(event): void => {
												const on =
													event.currentTarget.checked
												editRequirements((drafts) =>
													drafts.map((d, i) =>
														i !== index
															? d
															: {
																	...d,
																	category_ids:
																		on
																			? [
																					...d.category_ids,
																					category.id,
																				]
																			: d.category_ids.filter(
																					(
																						id,
																					) =>
																						id !==
																						category.id,
																				),
																},
													),
												)
											}}
										/>
										{category.id}
									</Label>
								))}
							</TableCell>
							<TableCell>
								<Input
									type="text"
									inputMode="numeric"
									className="w-20"
									aria-label="Periods required"
									value={requirement.count}
									onChange={(event): void => {
										const count = event.currentTarget.value
										editRequirements((drafts) =>
											drafts.map((d, i) =>
												i === index
													? { ...d, count }
													: d,
											),
										)
									}}
								/>
							</TableCell>
							<TableCell className="text-right">
								<Button
									type="button"
									variant="ghost"
									size="icon"
									className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
									aria-label="Remove this requirement"
									tooltip="Remove this requirement"
									onClick={(): void => {
										editRequirements((drafts) =>
											drafts.filter(
												(_, i) => i !== index,
											),
										)
									}}
								>
									<TrashIcon />
								</Button>
							</TableCell>
						</TableRow>
					))}
					{requirementDrafts.length === 0 && (
						<TableRow>
							<TableCell
								colSpan={3}
								className="text-muted-foreground"
							>
								No requirements.
							</TableCell>
						</TableRow>
					)}
				</TableBody>
			</Table>
			<div className="flex flex-wrap items-center gap-2">
				<Button
					type="button"
					variant="outline"
					size="sm"
					disabled={categories.length === 0}
					onClick={(): void => {
						editRequirements((drafts) => [
							...drafts,
							{ count: "1", category_ids: [] },
						])
					}}
				>
					Add requirement
				</Button>
				<Button
					type="button"
					size="sm"
					disabled={busy}
					onClick={(): void => {
						// Said out loud rather than returning quietly: a
						// button that does nothing is indistinguishable from
						// a broken one.
						if (requirementsMovedElsewhere) {
							setRequirementError(
								"Somebody else changed these requirements while " +
									"you were editing. Revert to see theirs, then " +
									"make your change again.",
							)
							return
						}
						const read = readRequirements(requirementDrafts)
						if (!read.ok) {
							setRequirementError(read.why)
							return
						}
						setRequirementError(null)
						void run(async (): Promise<void> => {
							await setGradeRequirements(grade.id, read.value)
							setTouched(null)
						})
					}}
				>
					Save requirements
				</Button>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					onClick={(): void => {
						setTouched(null)
						setRequirementError(null)
					}}
				>
					Revert
				</Button>
				{touched !== null && (
					<span className="text-xs text-muted-foreground">
						Unsaved changes
					</span>
				)}
			</div>
			{requirementError !== null && (
				<p role="alert" className="text-sm text-destructive">
					{requirementError}
				</p>
			)}
		</fieldset>
	)

	return (
		<article
			aria-labelledby={`${uid}-title`}
			className="card flex flex-col gap-4 rounded-xl border bg-card p-5 text-card-foreground shadow-xs"
		>
			<header className="flex flex-col gap-1">
				<div className="flex flex-wrap items-center gap-x-3 gap-y-2">
					<h3
						id={`${uid}-title`}
						className="text-lg font-semibold tracking-tight"
					>
						{grade.id}
					</h3>
					{/*
						Reads as the grade's title until it is pointed at:
						the border only appears on hover and focus, so a
						card of settings does not open with an empty-looking
						form field.
					*/}
					<Input
						type="text"
						className="h-8 w-48 border-transparent! bg-transparent px-2 shadow-none! hover:border-input! focus-visible:border-ring! dark:bg-transparent"
						aria-label={`Name of ${grade.id}`}
						value={nameDraft ?? grade.name}
						onChange={(event): void => {
							setNameDraft(event.currentTarget.value)
						}}
						onBlur={saveName}
					/>
					<SaveStatus state={nameState} />
					<div className="ml-auto flex items-center gap-1">
						<Button
							variant="ghost"
							size="sm"
							aria-label={`View students in ${grade.id}`}
							onClick={(): void => {
								window.location.hash = `#/students?q=${encodeURIComponent(
									`grade:${grade.id}`,
								)}`
							}}
						>
							Students
						</Button>
						<div className="flex items-center rounded-md border">
							<Button
								variant="ghost"
								size="icon-sm"
								className="rounded-r-none"
								aria-label={`Move ${grade.id} up`}
								disabled={busy}
								onClick={(): void => {
									move("up")
								}}
							>
								<Chevron direction="up" />
							</Button>
							<Button
								variant="ghost"
								size="icon-sm"
								className="rounded-l-none border-l"
								aria-label={`Move ${grade.id} down`}
								disabled={busy}
								onClick={(): void => {
									move("down")
								}}
							>
								<Chevron direction="down" />
							</Button>
						</div>
						<ConfirmDelete
							action="Delete"
							target={grade.id}
							detail="Its window, limits and requirements go with it. This cannot be undone."
							disabled={busy}
							onconfirm={(): void => {
								void run((): Promise<void> =>
									deleteGrade(grade.id),
								)
							}}
						/>
					</div>
				</div>
				{/*
					The summary is the status, in words, for everybody; the
					dot only repeats it for the eye.
				*/}
				<p className="flex items-start gap-2 text-sm text-muted-foreground">
					<span
						aria-hidden="true"
						className={
							grade.is_open
								? "mt-1.5 size-2 shrink-0 rounded-full bg-primary"
								: "mt-1.5 size-2 shrink-0 rounded-full bg-muted-foreground/40"
						}
					/>
					{windowSummary(grade)}
				</p>
			</header>

			{/*
				All three sections at once, each under its own heading: a
				grade's settings are few enough to read in one pass, and
				nothing about one of them is hidden behind another.
			*/}
			<div className="flex flex-col divide-y border-t [&>*]:py-4 [&>*:last-child]:pb-0">
				{/* Wrapped: a legend ignores its fieldset's top padding. */}
				<div>{windowSection}</div>
				<div>{limitsSection}</div>
				<div>{requirementsSection}</div>
			</div>
		</article>
	)
}
