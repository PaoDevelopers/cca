import { closeAllWindows, createGrade, setGradeOrder } from "@common/adminApi"
import { Button } from "@common/components/ui/button"
import { Input, Label } from "@common/components/ui/input"
import { toast } from "@common/components/ui/toast"
import { useEffect, useState, type ReactElement } from "react"
import { DataTools } from "./DataTools"
import { useAdminData } from "./data"
import { GradeCard } from "./GradeCard"
import { reordered } from "./ordering"

export function GradesPage(): ReactElement {
	const data = useAdminData()

	useEffect((): void => {
		data.want("grades", "categories")
	}, [data])

	const [newID, setNewID] = useState("")
	const [newName, setNewName] = useState("")

	const run = (action: () => Promise<void>): Promise<boolean> =>
		data.run(action, "grades")

	// The budget cap is the one grade setting that can invalidate an
	// enrollment, so it is the one that goes through the accept
	// protocol. Everything else is plain DML.
	const runAccepting = (
		action: (accept: string[]) => Promise<void>,
	): Promise<boolean> => data.runAccepting(action, "grades", "students")

	const move = (id: string, direction: "up" | "down"): void => {
		const ids = data.grades.map((g): string => g.id)
		void run((): Promise<void> =>
			setGradeOrder(reordered(ids, id, direction)),
		)
	}

	return (
		<section
			aria-labelledby="grades-heading"
			className="flex flex-col gap-6"
		>
			<div className="flex flex-wrap items-end justify-between gap-4">
				<div className="flex flex-col gap-1">
					{/*
						tabIndex because it is also where the data layer puts
						the focus when the control that started a write no
						longer exists.
					*/}
					<h2
						id="grades-heading"
						tabIndex={-1}
						className="text-2xl font-semibold tracking-tight outline-none"
					>
						Grades
					</h2>
					<p className="text-sm text-muted-foreground">
						Enrollment windows, period budgets and requirements, per
						grade.
					</p>
				</div>
				{data.ready("grades", "categories") && (
					<div className="flex items-center gap-2">
						<Button
							type="button"
							variant="outline"
							size="sm"
							title="Ends enrollment everywhere at once. Schedules are kept: a grade that has not opened yet is left as it is."
							disabled={
								data.busy ||
								!data.grades.some((g): boolean => g.is_open)
							}
							onClick={(): void => {
								void run((): Promise<void> =>
									closeAllWindows(),
								).then((ok): void => {
									if (ok) {
										toast("Closed every open window.")
									}
								})
							}}
						>
							Close every open window
						</Button>
						<DataTools
							section="grades"
							busy={data.busy}
							run={run}
						/>
					</div>
				)}
			</div>

			{!data.ready("grades", "categories") ? (
				<p className="text-muted-foreground">Loading…</p>
			) : (
				<>
					<div className="flex flex-col divide-y overflow-hidden rounded-xl border bg-card shadow-xs">
						{data.grades.map((grade) => (
							<GradeCard
								key={grade.id}
								grade={grade}
								categories={data.categories}
								busy={data.busy}
								run={run}
								runAccepting={runAccepting}
								move={(direction): void => {
									move(grade.id, direction)
								}}
							/>
						))}
						{data.grades.length === 0 && (
							<p className="px-4 py-3 text-muted-foreground">
								No grades yet.
							</p>
						)}
					</div>

					<form
						className="flex flex-wrap items-end gap-3 rounded-xl border border-dashed p-4"
						onSubmit={(event): void => {
							event.preventDefault()
							const id = newID.trim()
							const name = newName.trim()
							if (id === "" || name === "") {
								return
							}
							// Created closed, with no cap: opening a window and
							// setting a budget are separate, deliberate acts.
							void run(async (): Promise<void> => {
								await createGrade(id, name, null, 0)
								setNewID("")
								setNewName("")
								toast(`Added grade ${id}.`)
							})
						}}
					>
						<Label className="flex-col items-start">
							New grade ID
							<Input
								type="text"
								className="w-40"
								required
								value={newID}
								onChange={(event): void => {
									setNewID(event.currentTarget.value)
								}}
							/>
						</Label>
						<Label className="flex-col items-start">
							Name
							<Input
								type="text"
								className="w-56"
								required
								value={newName}
								onChange={(event): void => {
									setNewName(event.currentTarget.value)
								}}
							/>
						</Label>
						<Button size="default" disabled={data.busy}>
							Add
						</Button>
					</form>
				</>
			)}
		</section>
	)
}
