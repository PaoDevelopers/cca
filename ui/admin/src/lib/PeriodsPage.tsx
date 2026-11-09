import {
	createPeriod,
	deletePeriod,
	renamePeriod,
	setPeriodOrder,
} from "@common/adminApi"
import { Button } from "@common/components/ui/button"
import { Input, Label } from "@common/components/ui/input"
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@common/components/ui/table"
import { toast } from "@common/components/ui/toast"
import { useEffect, useState, type ReactElement } from "react"
import { ConfirmDelete } from "./ConfirmDelete"
import { DataTools } from "./DataTools"
import { useAdminData } from "./data"
import { reordered } from "./ordering"

export function PeriodsPage(): ReactElement {
	const data = useAdminData()
	useEffect((): void => {
		data.want("periods")
	}, [data])

	const [newID, setNewID] = useState("")
	const [newName, setNewName] = useState("")

	function move(id: string, direction: "up" | "down"): void {
		const ids = data.periods.map((p): string => p.id)
		void run(() => setPeriodOrder(reordered(ids, id, direction)))
	}

	const run = (action: () => Promise<void>): Promise<boolean> =>
		data.run(action, "periods")

	return (
		<section
			aria-labelledby="periods-heading"
			className="flex flex-col gap-6"
		>
			<div className="flex flex-wrap items-end justify-between gap-4">
				<div>
					{/*
						tabIndex because it is also where the data layer
						puts the focus when the control that started a
						write no longer exists.
					*/}
					<h2
						id="periods-heading"
						tabIndex={-1}
						className="text-2xl font-semibold tracking-tight outline-none"
					>
						Periods
					</h2>
					<p className="text-sm text-muted-foreground">
						The meeting slots courses are scheduled into, in display
						order.
					</p>
				</div>
			</div>

			<DataTools section="periods" busy={data.busy} run={run} />

			{!data.ready("periods") ? (
				<p>Loading…</p>
			) : (
				<>
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead scope="col">ID</TableHead>
								<TableHead scope="col">Name</TableHead>
								<TableHead scope="col" className="text-right">
									Actions
								</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{data.periods.map((period) => (
								<TableRow key={period.id}>
									<TableHead
										scope="row"
										className="font-mono text-foreground"
									>
										{period.id}
									</TableHead>
									<TableCell>
										{/*
											The name is inert prose: it may carry a
											wall-clock time, but nothing computes with
											it. Committed on blur.
											Keyed by the name so a rename from anywhere
											resets the box.
										*/}
										<Input
											key={period.name}
											type="text"
											className="max-w-sm"
											aria-label={`Name of ${period.id}`}
											defaultValue={period.name}
											onKeyDown={(event): void => {
												if (event.key === "Enter") {
													event.currentTarget.blur()
												}
											}}
											onBlur={(event): void => {
												const name =
													event.currentTarget.value.trim()
												if (
													name === "" ||
													name === period.name
												) {
													return
												}
												void run(() =>
													renamePeriod(
														period.id,
														name,
													),
												)
											}}
										/>
									</TableCell>
									<TableCell>
										<div className="flex justify-end gap-1">
											<Button
												variant="ghost"
												size="sm"
												aria-label={`View courses meeting in ${period.id}`}
												onClick={(): void => {
													window.location.hash = `#/courses?q=${encodeURIComponent(
														`period:${period.id}`,
													)}`
												}}
											>
												Courses
											</Button>
											<Button
												variant="ghost"
												size="icon-sm"
												aria-label={`Move ${period.id} up`}
												disabled={data.busy}
												onClick={(): void => {
													move(period.id, "up")
												}}
											>
												↑
											</Button>
											<Button
												variant="ghost"
												size="icon-sm"
												aria-label={`Move ${period.id} down`}
												disabled={data.busy}
												onClick={(): void => {
													move(period.id, "down")
												}}
											>
												↓
											</Button>
											<ConfirmDelete
												action="Delete"
												target={period.id}
												disabled={data.busy}
												onconfirm={(): void => {
													void run(() =>
														deletePeriod(period.id),
													).then((ok): void => {
														if (ok) {
															toast(
																`Deleted period ${period.id}.`,
															)
														}
													})
												}}
											/>
										</div>
									</TableCell>
								</TableRow>
							))}
							{data.periods.length === 0 && (
								<TableRow>
									<TableCell
										colSpan={3}
										className="text-muted-foreground"
									>
										No periods yet.
									</TableCell>
								</TableRow>
							)}
						</TableBody>
					</Table>

					<form
						className="flex flex-wrap items-end gap-3 rounded-lg border p-4"
						onSubmit={(event): void => {
							event.preventDefault()
							const id = newID.trim()
							const name = newName.trim()
							if (id === "" || name === "") {
								return
							}
							void run(async (): Promise<void> => {
								await createPeriod(id, name)
								setNewID("")
								setNewName("")
								toast(`Added period ${id}.`)
							})
						}}
					>
						<Label className="flex-col items-start">
							New period ID
							<Input
								type="text"
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
