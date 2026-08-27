// The shape of the student page. Everything it shows comes from
// useStudentData; nothing here reads the network or judges a rule.
import { useMemo, useState, type ReactElement, type ReactNode } from "react"
import { ChevronDown, LogOut } from "lucide-react"
import { CourseList } from "./components/CourseList"
import { ErrorPopup } from "./components/ErrorPopup"
import { Footer } from "./components/Footer"
import { HomePage } from "./components/HomePage"
import { Sidebar } from "./components/Sidebar"
import { ViewToggle } from "./components/ViewToggle"
import { Button } from "@/components/ui/button"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { courseRows, type CourseActions } from "./lib/enrollment"
import { useStudentData } from "./lib/useStudentData"
import { cn } from "./lib/utils"

type Page = "home" | "mine" | "available"

// Links that leave the app. They read as tabs because that is where
// people look for help, but nothing here can ever be the current page.
const support = [
	{ label: "Chat Support", href: "https://webirc.runxiyu.org/kiwiirc/#chat" },
	{
		label: "Email Us",
		href: "mailto:sj-cca@ykpaoschool.cn?cc=runxiyu@umich.edu,me@runxiyu.org,s23321@stu.ykpaoschool.cn",
	},
]

// Only a link that actually navigates gets a new tab. A mailto: handed
// to target="_blank" opens the mail client and leaves an about:blank
// tab sitting there, which is worse than not opening a tab at all.
function opensTab(href: string): boolean {
	return href.startsWith("http")
}

const tabClass =
	"whitespace-nowrap border-b-2 border-transparent px-3 py-3 text-sm text-muted-foreground hover:text-foreground"

// Both list pages open the same way: the heading that names the list,
// and the control for how it is drawn. tabindex because the heading is
// also where the focus lands when a write empties the list.
function PageHeading({
	id,
	children,
	view,
	onview,
}: {
	id: string
	children: string
	view: "cards" | "table"
	onview: (view: "cards" | "table") => void
}): ReactElement {
	return (
		<div className="mb-6 flex items-center justify-between gap-4">
			<h2 id={id} tabIndex={-1} className="text-2xl">
				{children}
			</h2>
			<ViewToggle view={view} onview={onview} />
		</div>
	)
}

export function App(): ReactElement {
	const data = useStudentData()
	const [page, setPage] = useState<Page>("home")
	// One preference covers both lists.
	const [view, setView] = useState<"cards" | "table">("cards")

	const { enrollmentOf, violationsOf, canSwap, enroll, drop, swap } = data

	const actions: CourseActions = {
		windowOpen: data.windowOpen,
		updating: data.updating,
		onenroll: enroll,
		ondrop: drop,
	}

	const catalogueRows = useMemo(
		() => courseRows(data.catalogue, enrollmentOf, violationsOf, canSwap),
		[data.catalogue, enrollmentOf, violationsOf, canSwap],
	)

	// The student's own list offers no swap: there is nothing to swap
	// into a course they already hold.
	const selectedRows = useMemo(
		() =>
			courseRows(
				data.selected,
				enrollmentOf,
				(): [] => [],
				(): boolean => false,
			),
		[data.selected, enrollmentOf],
	)

	// Browse before review, which is the order a student works in. The
	// counts are in the accessible name and not the visible label: a
	// sighted student is one click from the list itself, and somebody
	// hearing the page cannot glance at it.
	const tabs: Array<{ key: Page; label: string; count: number | null }> = [
		{ key: "home", label: "Home", count: null },
		{ key: "available", label: "All courses", count: catalogueRows.length },
		{
			key: "mine",
			label: "Your selections",
			count: selectedRows.length,
		},
	]

	function go(next: Page): void {
		setPage(next)
		if (next !== "home") {
			data.forgetFocus()
		}
	}

	let content: ReactNode
	if (data.unauthenticated) {
		content = (
			<p role="alert">
				You are not signed in, or your session has expired.{" "}
				<a className="underline underline-offset-2" href="/student/">
					Sign in
				</a>
			</p>
		)
	} else if (data.loading) {
		content = <p>Loading&hellip;</p>
	} else if (page === "home") {
		content = (
			<HomePage
				user={data.user}
				grade={data.grade}
				categories={data.categories}
			/>
		)
	} else if (page === "mine") {
		content = (
			<section aria-labelledby="mine-heading">
				<PageHeading id="mine-heading" view={view} onview={setView}>
					Your selections
				</PageHeading>
				<CourseList
					rows={selectedRows}
					categories={data.categories}
					empty="You have not selected any courses."
					view={view}
					actions={actions}
				/>
			</section>
		)
	} else {
		content = (
			<div className="flex flex-col gap-6 md:flex-row md:gap-8">
				<Sidebar
					filter={data.filter}
					categories={data.courseCategories}
					periods={data.periods}
					onchange={data.setFilter}
				/>
				<section
					aria-labelledby="available-heading"
					className="min-w-0 flex-1"
				>
					<PageHeading
						id="available-heading"
						view={view}
						onview={setView}
					>
						All courses
					</PageHeading>
					<CourseList
						rows={catalogueRows}
						categories={data.courseCategories}
						empty="No courses match your search."
						view={view}
						actions={{ ...actions, onswap: swap }}
					/>
				</section>
			</div>
		)
	}

	return (
		<>
			{/*
				Polite, so it waits for a pause rather than cutting across
				whatever the student is reading, and outside <main> so that
				re-rendering a page cannot take it away mid-announcement.
			*/}
			<p aria-live="polite" className="for-screen-readers">
				{data.announcement}
			</p>

			<header className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 border-b px-6 py-4">
				<h1 className="text-xl">YKPao CCA Selection</h1>
				{data.user !== null && (
					<>
						{/*
							Outside the menu, and submitted from inside it
							through the button's form attribute: the menu
							content is portalled to the end of <body>, so a
							form wrapped around the item would not survive
							the move.
						*/}
						<form
							id="logout"
							method="post"
							action="/student/logout"
							className="hidden"
						/>
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								{/*
									The whole identity line triggers the menu.
									Who is signed in belongs on the page, not
									behind a click — on a shared machine it is
									the first thing worth checking — so the
									menu holds only the one thing a menu is
									for.
								*/}
								<Button variant="ghost" size="sm">
									<span className="font-medium">
										{data.user.name}
									</span>
									<span className="font-normal text-muted-foreground">
										{data.user.grade_id}
									</span>
									<span className="font-normal text-muted-foreground">
										{data.user.id}
									</span>
									<ChevronDown aria-hidden="true" />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								<DropdownMenuItem asChild>
									<button
										type="submit"
										form="logout"
										className="w-full cursor-pointer"
									>
										<LogOut aria-hidden="true" />
										Sign out
									</button>
								</DropdownMenuItem>
							</DropdownMenuContent>
						</DropdownMenu>
					</>
				)}
			</header>

			{!data.unauthenticated && !data.loading && (
				<div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b px-6">
					{/*
						Buttons rather than links: these switch in-page
						state and have no URL of their own. aria-current
						marks the active one; the tab look is presentation.
					*/}
					<nav className="-mb-px flex min-w-0 gap-1 overflow-x-auto">
						{tabs.map((tab): ReactElement => (
							<button
								key={tab.key}
								aria-current={
									page === tab.key ? "page" : undefined
								}
								aria-label={
									tab.count === null
										? undefined
										: `${tab.label} (${String(tab.count)})`
								}
								className={cn(
									tabClass,
									"cursor-pointer",
									page === tab.key &&
										"border-primary font-medium text-foreground",
								)}
								onClick={(): void => {
									go(tab.key)
								}}
							>
								{tab.label}
							</button>
						))}
					</nav>
					<nav
						aria-label="Support"
						className="-mb-px flex min-w-0 gap-1 overflow-x-auto"
					>
						{support.map((link): ReactElement => (
							<a
								key={link.label}
								href={link.href}
								{...(opensTab(link.href)
									? {
											target: "_blank",
											rel: "noreferrer",
										}
									: {})}
								className={tabClass}
							>
								{link.label}
							</a>
						))}
					</nav>
				</div>
			)}

			<main className="flex-1 px-6 py-8">{content}</main>

			<Footer />

			<ErrorPopup message={data.error} ondismiss={data.dismissError} />
		</>
	)
}
