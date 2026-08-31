// The portal: two doors, and which of them you are already through.
//
// It belongs to neither panel. The card markup is written out rather
// than pulled from the student panel's component directory for the same
// reason the build is separate — the page that chooses between two
// areas should not be built out of one of them. The look is shared
// through the tokens in common/styles/theme.css, which is the part that
// actually has to match.
import "./index.css"
import { StrictMode, useEffect, useState, type ReactElement } from "react"
import { createRoot } from "react-dom/client"
import { getJSON } from "@common/http"

// Who the caller is signed in as, in each area; empty where they are
// not. It matches sessionNames in internal/web/index.go.
interface Session {
	student: string
	admin: string
}

const areas: Array<{
	href: string
	title: string
	blurb: string
	key: keyof Session
}> = [
	{
		href: "/student/",
		title: "Student",
		blurb: "Browse the CCAs and manage your selections.",
		key: "student",
	},
	{
		href: "/admin/",
		title: "Administrator",
		blurb: "Manage courses, grades, students and enrolments.",
		key: "admin",
	},
]

function Portal(): ReactElement {
	const [session, setSession] = useState<Session | null>(null)

	useEffect((): void => {
		getJSON<Session>("/api/session")
			.then(setSession)
			.catch((): void => {
				// A name beside a door is decoration, and both doors
				// open signed in or out. There is nothing to tell
				// somebody here that they could act on.
			})
	}, [])

	return (
		<>
			{/*
				Centred, with no header bar over it: a header is the chrome
				of an app you are inside, and nobody is inside anything
				yet. The two choices are the whole page, so the page is
				laid out around them.
			*/}
			<main className="flex flex-1 items-center justify-center px-4 py-12 sm:px-6">
				<div className="flex w-full max-w-3xl flex-col gap-6">
					<h1 className="text-2xl font-semibold">
						YK Pao School CCA
					</h1>

					<div className="grid gap-4 sm:grid-cols-2">
						{areas.map((area): ReactElement => {
							const name =
								session === null ? "" : session[area.key]

							return (
								// The whole card is the link, so the target
								// is the card and not the one word in it.
								<a
									key={area.href}
									href={area.href}
									className="flex flex-col gap-2 rounded-xl border bg-card px-4 py-4 text-card-foreground outline-none transition-colors hover:border-primary hover:bg-accent focus-visible:ring-[3px] focus-visible:ring-ring/50"
								>
									<div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
										<h2 className="text-lg leading-none font-semibold">
											{area.title}
										</h2>
										{name !== "" && (
											<span className="text-xs text-muted-foreground">
												Signed in as {name}
											</span>
										)}
									</div>
									<p className="text-sm text-muted-foreground">
										{area.blurb}
									</p>
								</a>
							)
						})}
					</div>
				</div>
			</main>

			{/*
				The same notice the panels and the sign-in pages carry.
				Each frontend spells it in its own language already; this
				is the fourth and last of them.
			*/}
			<footer className="border-t px-4 py-3 text-xs text-muted-foreground sm:px-6">
				<a
					className="underline underline-offset-2 hover:text-foreground"
					href="https://github.com/PaoDevelopers/cca"
				>
					YK Pao School CCAs
				</a>{" "}
				is licensed under{" "}
				<a
					className="underline underline-offset-2 hover:text-foreground"
					href="https://spdx.org/licenses/AGPL-3.0-only.html"
				>
					GNU Affero General Public License v3.0 only
				</a>
				.
			</footer>
		</>
	)
}

const target = document.getElementById("app")

if (target === null) {
	throw new Error("App mount point missing")
}

createRoot(target).render(
	<StrictMode>
		<Portal />
	</StrictMode>,
)
