import { ErrorPopup } from "@common/components/ErrorPopup"
import { Footer } from "@common/components/Footer"
import { Button } from "@common/components/ui/button"
import { Toaster } from "@common/components/ui/toast"
import { errorMessage, getJSON } from "@common/http"
import { cn } from "@common/lib/utils"
import {
	Component,
	useEffect,
	useState,
	type ReactElement,
	type ReactNode,
} from "react"
import { CategoriesPage } from "./lib/CategoriesPage"
import { CoursesPage } from "./lib/CoursesPage"
import { AdminData, AdminDataContext, useAdminData } from "./lib/data"
import { EnrollmentsPage } from "./lib/EnrollmentsPage"
import { GradesPage } from "./lib/GradesPage"
import { PeriodsPage } from "./lib/PeriodsPage"
import { SignOutIcon } from "./lib/icons"
import { StudentsPage } from "./lib/StudentsPage"
import { ViolationDialog } from "./lib/ViolationDialog"

const pages = {
	"#/": "Home",
	"#/categories": "Categories",
	"#/periods": "Periods",
	"#/grades": "Grades",
	"#/courses": "Courses",
	"#/students": "Students",
	"#/enrollments": "Enrollments",
} as const
type Route = keyof typeof pages

// The hash may carry a query after "?", e.g. "#/enrollments?q=…". q is
// the filter box's value, read from the hash rather than copied out of
// it, so landing on a link, clearing the box and clicking the current
// tab all agree without anything having to be synced.
function parseHash(): { route: Route; query: string } {
	const hash = window.location.hash
	const cut = hash.indexOf("?")
	const base = cut === -1 ? hash : hash.slice(0, cut)
	const params = new URLSearchParams(cut === -1 ? "" : hash.slice(cut + 1))
	return {
		route: base in pages ? (base as Route) : "#/",
		query: params.get("q") ?? "",
	}
}

// One data layer and one event socket for the whole app; the pages
// read it out of context.
const adminData = new AdminData()

export function App(): ReactElement {
	useEffect((): (() => void) => adminData.connect(), [])

	return (
		<AdminDataContext value={adminData}>
			<Shell />
		</AdminDataContext>
	)
}

function Shell(): ReactElement {
	const data = useAdminData()
	const [{ route, query }, setLocation] = useState(parseHash)
	// Who is signed in, for the foot of the sidebar. A decoration: if
	// the lookup fails, the name is simply not shown.
	const [account, setAccount] = useState<string | null>(null)

	useEffect((): void => {
		getJSON<{ admin: string }>("/api/session").then(
			(session): void => {
				setAccount(session.admin === "" ? null : session.admin)
			},
			(): void => {
				setAccount(null)
			},
		)
	}, [])

	useEffect((): (() => void) => {
		const onhashchange = (): void => {
			setLocation(parseHash())
		}
		window.addEventListener("hashchange", onhashchange)
		return (): void => {
			window.removeEventListener("hashchange", onhashchange)
		}
	}, [])

	// Typing leaves the URL alone. Clearing drops the query from it,
	// so a reload does not bring back the filter that was just removed.
	const setFilter = (next: string): void => {
		if (next === "" && window.location.hash.includes("?")) {
			history.replaceState(null, "", route)
		}
		setLocation({ route, query: next })
	}

	return (
		<>
			<div className="flex flex-1 flex-col md:flex-row">
				<aside className="z-20 flex shrink-0 flex-wrap items-center gap-x-4 gap-y-2 border-b bg-muted/40 px-4 py-3 md:sticky md:top-0 md:h-svh md:w-56 md:flex-col md:flex-nowrap md:items-stretch md:border-r md:border-b-0 md:px-3 md:py-5">
					<h1 className="text-lg font-semibold tracking-tight md:px-3 md:pb-3">
						CCA Admin Panel
					</h1>
					{/*
						Link-based navigation: each entry changes the hash
						route, so <nav> plus aria-current="page" is the
						accurate markup.
					*/}
					<nav className="order-last flex w-full min-w-0 gap-1 overflow-x-auto md:order-none md:flex-col md:overflow-visible">
						{Object.entries(pages).map(([hash, label]) => (
							<a
								key={hash}
								href={hash}
								aria-current={
									route === hash ? "page" : undefined
								}
								className={cn(
									"rounded-md px-3 py-2 text-sm font-medium whitespace-nowrap text-muted-foreground transition-colors hover:bg-accent hover:text-foreground",
									"aria-[current=page]:bg-accent aria-[current=page]:text-foreground",
								)}
							>
								{label}
							</a>
						))}
					</nav>
					<div className="ml-auto flex min-w-0 items-center gap-1 md:mt-auto md:ml-0 md:border-t md:pt-3 md:pl-3">
						{account !== null && (
							<span
								className="min-w-0 flex-1 truncate text-sm"
								title={account}
							>
								<span className="sr-only">Signed in as </span>
								{account}
							</span>
						)}
						<form method="post" action="/admin/logout">
							<Button
								type="submit"
								variant="outline"
								size="icon"
								className="text-muted-foreground hover:text-foreground"
								aria-label="Sign out"
								tooltip="Sign out"
							>
								<SignOutIcon />
							</Button>
						</form>
					</div>
				</aside>

				<div className="flex min-w-0 flex-1 flex-col">
					{/*
						tabIndex so the data layer has somewhere to put the
						focus when the control that started a write no longer
						exists — a save that closes its own editor, a delete
						that removes its own row. Landing here continues the
						tab order inside the page rather than restarting it at
						the top of the document.
					*/}
					<main
						tabIndex={-1}
						className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 outline-none sm:px-6"
					>
						{/*
							Keyed by route so a page that failed to render gets
							a fresh boundary, and a fresh chance, on the next
							tab.
						*/}
						<Boundary key={route}>
							{data.unauthenticated ? (
								<p role="alert">
									You are not signed in, or your session has
									expired.{" "}
									<a className="underline" href="/admin/">
										Sign in
									</a>
								</p>
							) : (
								<Page
									route={route}
									filter={query}
									setFilter={setFilter}
								/>
							)}
						</Boundary>
					</main>

					<Footer />
				</div>
			</div>

			<ErrorPopup
				message={data.error}
				ondismiss={(): void => {
					data.dismissError()
				}}
			/>

			<Toaster />

			{/*
				Outside the boundary and outside every page: any negotiable
				write can raise it, and the answer has to survive the page
				it came from.
			*/}
			{data.pendingViolations !== null && (
				<ViolationDialog
					violations={data.pendingViolations.violations}
					answer={data.pendingViolations.answer}
				/>
			)}
		</>
	)
}

function Page({
	route,
	filter,
	setFilter,
}: {
	route: Route
	filter: string
	setFilter: (filter: string) => void
}): ReactElement {
	switch (route) {
		case "#/categories":
			return <CategoriesPage />
		case "#/periods":
			return <PeriodsPage />
		case "#/grades":
			return <GradesPage />
		case "#/courses":
			return <CoursesPage filter={filter} setFilter={setFilter} />
		case "#/students":
			return <StudentsPage filter={filter} setFilter={setFilter} />
		case "#/enrollments":
			return <EnrollmentsPage filter={filter} setFilter={setFilter} />
		case "#/":
			return <p className="text-muted-foreground">Choose a section.</p>
	}
}

// A render error would otherwise unmount the whole app and leave a
// blank page with nothing to say why. The boundary replaces the section
// instead, so the failure is visible and recoverable. It catches
// rendering only — failed requests are ApiErrors and go to the popover.
class Boundary extends Component<
	{ children: ReactNode },
	{ error: unknown; failed: boolean }
> {
	public override state = { error: null as unknown, failed: false }

	public static getDerivedStateFromError(error: unknown): {
		error: unknown
		failed: boolean
	} {
		return { error, failed: true }
	}

	public override render(): ReactNode {
		if (!this.state.failed) {
			return this.props.children
		}
		return (
			<p role="alert" className="flex items-center gap-3">
				This section could not be displayed:{" "}
				{errorMessage(this.state.error, "unexpected error")}
				<Button
					variant="outline"
					size="sm"
					onClick={(): void => {
						this.setState({ error: null, failed: false })
					}}
				>
					Try again
				</Button>
			</p>
		)
	}
}
