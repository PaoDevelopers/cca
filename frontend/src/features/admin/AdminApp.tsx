import { useCallback, useEffect, useState } from "react"
import {
	BellIcon,
	BookOpenIcon,
	CalendarDaysIcon,
	ChartNoAxesCombinedIcon,
	ClipboardListIcon,
	DatabaseIcon,
	FolderTreeIcon,
	GraduationCapIcon,
	UsersIcon,
} from "lucide-react"
import { Link, Navigate, Route, Routes, useLocation } from "react-router-dom"

import { apiRequest } from "@/api"
import { ErrorAlert, PageSkeleton } from "@/components/common"
import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import {
	Sidebar,
	SidebarContent,
	SidebarFooter,
	SidebarGroup,
	SidebarGroupContent,
	SidebarGroupLabel,
	SidebarHeader,
	SidebarInset,
	SidebarMenu,
	SidebarMenuButton,
	SidebarMenuItem,
	SidebarProvider,
	SidebarRail,
	SidebarTrigger,
	useSidebar,
} from "@/components/ui/sidebar"
import type { AdminBootstrap } from "@/types"
import {
	CategoriesPage,
	CoursesPage,
	DashboardPage,
	DataManagementPage,
	GradesPage,
	NotificationsPage,
	PeriodsPage,
	SelectionsPage,
	StudentsPage,
} from "@/features/admin/AdminPages"

const navigation = [
	{
		path: "/admin/dashboard",
		label: "Dashboard",
		icon: ChartNoAxesCombinedIcon,
	},
	{ path: "/admin/periods", label: "Periods", icon: CalendarDaysIcon },
	{ path: "/admin/categories", label: "Categories", icon: FolderTreeIcon },
	{ path: "/admin/grades", label: "Grades", icon: GraduationCapIcon },
	{ path: "/admin/courses", label: "Courses", icon: BookOpenIcon },
	{ path: "/admin/students", label: "Students", icon: UsersIcon },
	{ path: "/admin/selections", label: "Selections", icon: ClipboardListIcon },
	{ path: "/admin/notifications", label: "Notifications", icon: BellIcon },
	{ path: "/admin/data", label: "Data management", icon: DatabaseIcon },
] as const

export interface AdminPageProps {
	data: AdminBootstrap
	refresh: () => Promise<void>
}

function AdminNavigation({
	pathname,
}: {
	pathname: string
}): React.JSX.Element {
	const { setOpenMobile } = useSidebar()

	return (
		<nav aria-label="Administration">
			<SidebarMenu>
				{navigation.map((item) => {
					const Icon = item.icon
					return (
						<SidebarMenuItem key={item.path}>
							<SidebarMenuButton
								render={
									<Link
										to={item.path}
										onClick={() => setOpenMobile(false)}
									/>
								}
								isActive={pathname.startsWith(item.path)}
								tooltip={item.label}
							>
								<Icon aria-hidden="true" />
								<span>{item.label}</span>
							</SidebarMenuButton>
						</SidebarMenuItem>
					)
				})}
			</SidebarMenu>
		</nav>
	)
}

function AdminShell({
	children,
	data,
}: {
	children: React.ReactNode
	data: AdminBootstrap
}): React.JSX.Element {
	const location = useLocation()
	const current =
		navigation.find((item) => location.pathname.startsWith(item.path)) ??
		navigation[0]
	return (
		<SidebarProvider>
			<Sidebar collapsible="icon">
				<SidebarHeader>
					<div className="flex h-10 items-center gap-2 px-2 group-data-[collapsible=icon]:justify-center">
						<div className="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
							<BookOpenIcon aria-hidden="true" />
						</div>
						<div className="min-w-0 group-data-[collapsible=icon]:hidden">
							<p className="truncate font-heading font-semibold">
								CCA Admin
							</p>
							<p className="truncate text-xs text-muted-foreground">
								YK Pao School
							</p>
						</div>
					</div>
				</SidebarHeader>
				<SidebarContent>
					<SidebarGroup>
						<SidebarGroupLabel>Management</SidebarGroupLabel>
						<SidebarGroupContent>
							<AdminNavigation pathname={location.pathname} />
						</SidebarGroupContent>
					</SidebarGroup>
				</SidebarContent>
				<SidebarFooter>
					<div className="flex items-center gap-2 px-2 py-1 group-data-[collapsible=icon]:justify-center">
						<div className="flex size-8 items-center justify-center rounded-lg bg-muted font-medium">
							{data.admin.username.slice(0, 1).toUpperCase()}
						</div>
						<div className="min-w-0 group-data-[collapsible=icon]:hidden">
							<p className="truncate text-sm font-medium">
								{data.admin.username}
							</p>
							<p className="text-xs text-muted-foreground">
								Administrator
							</p>
						</div>
					</div>
				</SidebarFooter>
				<SidebarRail />
			</Sidebar>

			<SidebarInset id="main-content" className="min-w-0">
				<header className="sticky top-0 z-30 isolate flex h-14 items-center gap-3 border-b bg-background px-4 shadow-sm">
					<SidebarTrigger />
					<Separator orientation="vertical" className="h-5" />
					<div className="flex min-w-0 flex-1 items-center justify-between gap-3">
						<div className="min-w-0">
							<h1 className="truncate font-heading font-semibold">
								{current.label}
							</h1>
							<p className="hidden text-xs text-muted-foreground sm:block">
								Manage the CCA sign-up system.
							</p>
						</div>
						<Badge className="shrink-0" variant="outline">
							Schema v2
						</Badge>
					</div>
				</header>
				<div className="min-w-0 flex-1 p-4 sm:p-6">{children}</div>
			</SidebarInset>
		</SidebarProvider>
	)
}

export default function AdminApp(): React.JSX.Element {
	const [data, setData] = useState<AdminBootstrap | null>(null)
	const [error, setError] = useState<string | null>(null)

	const refresh = useCallback(async (): Promise<void> => {
		try {
			const next = await apiRequest<AdminBootstrap>(
				"/api/v1/admin/bootstrap",
			)
			setData(next)
			setError(null)
		} catch (caught) {
			setError(
				caught instanceof Error
					? caught.message
					: "Unable to load administration data.",
			)
		}
	}, [])

	useEffect(() => {
		// eslint-disable-next-line react-hooks/set-state-in-effect -- Bootstrap data is loaded after the request resolves.
		void refresh()
	}, [refresh])

	if (data === null && error === null) return <PageSkeleton />
	if (data === null)
		return (
			<main id="main-content" className="mx-auto max-w-3xl p-6">
				<ErrorAlert message={error ?? "Unable to load."} />
			</main>
		)

	const pageProps: AdminPageProps = { data, refresh }
	return (
		<AdminShell data={data}>
			{error !== null ? (
				<div className="mb-4">
					<ErrorAlert message={error} />
				</div>
			) : null}
			<Routes>
				<Route
					path="dashboard"
					element={<DashboardPage {...pageProps} />}
				/>
				<Route
					path="periods"
					element={<PeriodsPage {...pageProps} />}
				/>
				<Route
					path="categories"
					element={<CategoriesPage {...pageProps} />}
				/>
				<Route path="grades" element={<GradesPage {...pageProps} />} />
				<Route
					path="courses"
					element={<CoursesPage {...pageProps} />}
				/>
				<Route
					path="students"
					element={<StudentsPage {...pageProps} />}
				/>
				<Route
					path="selections"
					element={<SelectionsPage {...pageProps} />}
				/>
				<Route
					path="notifications"
					element={<NotificationsPage {...pageProps} />}
				/>
				<Route
					path="data"
					element={<DataManagementPage {...pageProps} />}
				/>
				<Route path="*" element={<Navigate to="dashboard" replace />} />
			</Routes>
		</AdminShell>
	)
}
