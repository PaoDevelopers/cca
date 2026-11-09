import {
	createCategory,
	deleteCategory,
	renameCategory,
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

export function CategoriesPage(): ReactElement {
	const data = useAdminData()
	useEffect((): void => {
		data.want("categories")
	}, [data])

	const [newID, setNewID] = useState("")
	const [newName, setNewName] = useState("")

	const run = (action: () => Promise<void>): Promise<boolean> =>
		data.run(action, "categories")

	return (
		<section
			aria-labelledby="categories-heading"
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
						id="categories-heading"
						tabIndex={-1}
						className="text-2xl font-semibold tracking-tight outline-none"
					>
						Categories
					</h2>
					<p className="text-sm text-muted-foreground">
						The kinds of activity a course belongs to.
					</p>
				</div>
				<DataTools section="categories" busy={data.busy} run={run} />
			</div>

			{!data.ready("categories") ? (
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
							{data.categories.map((category) => (
								<TableRow key={category.id}>
									<TableHead
										scope="row"
										className="font-mono text-foreground"
									>
										{category.id}
									</TableHead>
									<TableCell>
										{/*
											Committed on blur rather than on every
											keystroke: renaming is one intention, and
											a request per character would be several.
											Keyed by the name so a rename from anywhere
											resets the box.
										*/}
										<Input
											key={category.name}
											type="text"
											className="max-w-sm"
											aria-label={`Name of ${category.id}`}
											defaultValue={category.name}
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
													name === category.name
												) {
													return
												}
												void run(() =>
													renameCategory(
														category.id,
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
												aria-label={`View courses in ${category.id}`}
												onClick={(): void => {
													window.location.hash = `#/courses?q=${encodeURIComponent(
														`category:${category.id}`,
													)}`
												}}
											>
												Courses
											</Button>
											<ConfirmDelete
												action="Delete"
												target={category.id}
												disabled={data.busy}
												onconfirm={(): void => {
													void run(() =>
														deleteCategory(
															category.id,
														),
													).then((ok): void => {
														if (ok) {
															toast(
																`Deleted category ${category.id}.`,
															)
														}
													})
												}}
											/>
										</div>
									</TableCell>
								</TableRow>
							))}
							{data.categories.length === 0 && (
								<TableRow>
									<TableCell
										colSpan={3}
										className="text-muted-foreground"
									>
										No categories yet.
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
								await createCategory(id, name)
								setNewID("")
								setNewName("")
								toast(`Added category ${id}.`)
							})
						}}
					>
						<Label className="flex-col items-start">
							New category ID
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
