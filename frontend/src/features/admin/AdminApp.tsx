import { useCallback, useEffect, useState } from "react"
import {
	BellIcon,
	BookOpenIcon,
	CalendarDaysIcon,
	ChartNoAxesCombinedIcon,
	DatabaseIcon,
	FolderTreeIcon,
	GraduationCapIcon,
	ShieldUserIcon,
	type LucideIcon,
	UsersIcon,
} from "lucide-react"
import { Link, Navigate, Route, Routes, useLocation } from "react-router-dom"

import { apiRequest } from "@/api"
import { ErrorAlert, PageSkeleton } from "@/components/common"
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
	SidebarSeparator,
	SidebarTrigger,
	useSidebar,
} from "@/components/ui/sidebar"
import type { AdminBootstrap, AdminDashboard, AdminSession } from "@/types"
import {
	CategoriesPage,
	CoursesPage,
	DashboardPage,
	DataManagementPage,
	GradesPage,
	NotificationsPage,
	PeriodsPage,
} from "@/features/admin/AdminPages"
import { ParticipationPage } from "@/features/admin/ParticipationPage"

interface NavigationItem {
	path: string
	label: string
	icon: LucideIcon
}

interface NavigationSection {
	label: string
	items: readonly NavigationItem[]
}

const navigationSections: readonly NavigationSection[] = [
	{
		label: "Overview",
		items: [
			{
				path: "/admin/dashboard",
				label: "Dashboard",
				icon: ChartNoAxesCombinedIcon,
			},
		],
	},
	{
		label: "Configuration",
		items: [
			{
				path: "/admin/periods",
				label: "Periods",
				icon: CalendarDaysIcon,
			},
			{
				path: "/admin/categories",
				label: "Categories",
				icon: FolderTreeIcon,
			},
			{
				path: "/admin/grades",
				label: "Grades",
				icon: GraduationCapIcon,
			},
			{
				path: "/admin/courses",
				label: "Courses",
				icon: BookOpenIcon,
			},
		],
	},
	{
		label: "Participation",
		items: [
			{
				path: "/admin/participation",
				label: "Participation",
				icon: UsersIcon,
			},
		],
	},
	{
		label: "System",
		items: [
			{
				path: "/admin/notifications",
				label: "Notifications",
				icon: BellIcon,
			},
			{
				path: "/admin/data",
				label: "Data management",
				icon: DatabaseIcon,
			},
		],
	},
]

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
		<>
			{navigationSections.map((section) => (
				<SidebarGroup key={section.label}>
					<SidebarGroupLabel>{section.label}</SidebarGroupLabel>
					<SidebarGroupContent>
						<nav aria-label={section.label}>
							<SidebarMenu>
								{section.items.map((item) => {
									const Icon = item.icon
									const isActive = pathname.startsWith(
										item.path,
									)
									return (
										<SidebarMenuItem key={item.path}>
											<SidebarMenuButton
												render={
													<Link
														to={item.path}
														onClick={() =>
															setOpenMobile(false)
														}
														aria-current={
															isActive
																? "page"
																: undefined
														}
													/>
												}
												isActive={isActive}
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
					</SidebarGroupContent>
				</SidebarGroup>
			))}
		</>
	)
}

function AdminShell({
	children,
	admin,
}: {
	children: React.ReactNode
	admin: AdminSession
}): React.JSX.Element {
	const location = useLocation()
	return (
		<SidebarProvider>
			<Sidebar variant="sidebar" collapsible="icon">
				<SidebarHeader>
					<SidebarMenu>
						<SidebarMenuItem>
							<SidebarMenuButton
								render={<Link to="/admin/dashboard" />}
								size="lg"
								tooltip="CCA Admin"
							>
								<BookOpenIcon aria-hidden="true" />
								<span className="grid flex-1 text-left leading-tight group-data-[collapsible=icon]:hidden">
									<span className="truncate font-heading font-semibold">
										CCA Admin
									</span>
									<span className="truncate text-xs text-muted-foreground">
										YK Pao School
									</span>
								</span>
							</SidebarMenuButton>
						</SidebarMenuItem>
					</SidebarMenu>
				</SidebarHeader>
				<SidebarSeparator />
				<SidebarContent>
					<AdminNavigation pathname={location.pathname} />
				</SidebarContent>
				<SidebarSeparator />
				<SidebarFooter>
					<SidebarMenu>
						<SidebarMenuItem>
							<SidebarMenuButton
								size="lg"
								tooltip={admin.username}
							>
								<ShieldUserIcon aria-hidden="true" />
								<span className="grid flex-1 text-left leading-tight group-data-[collapsible=icon]:hidden">
									<span className="truncate font-medium">
										{admin.username}
									</span>
									<span className="truncate text-xs text-muted-foreground">
										Administrator
									</span>
								</span>
							</SidebarMenuButton>
						</SidebarMenuItem>
					</SidebarMenu>
				</SidebarFooter>
				<SidebarRail />
			</Sidebar>

			<SidebarInset id="main-content" className="min-w-0">
				<SidebarTrigger
					variant="outline"
					className="m-4 mb-0 md:hidden"
				/>
				<div className="min-w-0 flex-1 p-4 sm:p-6">{children}</div>
			</SidebarInset>
		</SidebarProvider>
	)
}

export default function AdminApp(): React.JSX.Element {
	const location = useLocation()
	const isAdminRoot =
		location.pathname === "/admin" || location.pathname === "/admin/"
	const isDashboard = location.pathname === "/admin/dashboard"
	const [data, setData] = useState<AdminBootstrap | null>(null)
	const [dashboard, setDashboard] = useState<AdminDashboard | null>(null)
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
		if (isAdminRoot) return
		if (isDashboard) {
			const controller = new AbortController()
			void apiRequest<AdminDashboard>("/api/v1/admin/dashboard", {
				signal: controller.signal,
			})
				.then((next) => {
					setDashboard(next)
					setError(null)
				})
				.catch((caught: unknown) => {
					if (controller.signal.aborted) return
					setError(
						caught instanceof Error
							? caught.message
							: "Unable to load dashboard data.",
					)
				})
			return () => controller.abort()
		}
		// eslint-disable-next-line react-hooks/set-state-in-effect -- Bootstrap data is loaded after the request resolves.
		void refresh()
	}, [isAdminRoot, isDashboard, refresh])

	if (isAdminRoot) return <Navigate to="dashboard" replace />

	if (isDashboard) {
		if (dashboard === null && error === null) return <PageSkeleton />
		if (dashboard === null)
			return (
				<main id="main-content" className="mx-auto max-w-3xl p-6">
					<ErrorAlert message={error ?? "Unable to load."} />
				</main>
			)
		return (
			<AdminShell admin={dashboard.admin}>
				{error !== null ? (
					<div className="mb-4">
						<ErrorAlert message={error} />
					</div>
				) : null}
				<DashboardPage data={dashboard} />
			</AdminShell>
		)
	}

	if (data === null && error === null) return <PageSkeleton />
	if (data === null)
		return (
			<main id="main-content" className="mx-auto max-w-3xl p-6">
				<ErrorAlert message={error ?? "Unable to load."} />
			</main>
		)

	const pageProps: AdminPageProps = { data, refresh }
	return (
		<AdminShell admin={data.admin}>
			{error !== null ? (
				<div className="mb-4">
					<ErrorAlert message={error} />
				</div>
			) : null}
			<Routes>
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
					path="participation"
					element={<ParticipationPage {...pageProps} />}
				/>
				<Route
					path="students"
					element={<Navigate to="../participation" replace />}
				/>
				<Route
					path="selections"
					element={<Navigate to="../participation" replace />}
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
