import { lazy, Suspense } from "react"
import { Navigate, Route, Routes } from "react-router-dom"

import { PageSkeleton } from "@/components/common"

const AdminApp = lazy(() => import("@/features/admin/app/AdminApp"))
const StudentApp = lazy(() => import("@/features/student/app/StudentApp"))
const TestLoginPage = lazy(() => import("@/features/auth/pages/TestLoginPage"))

export default function App(): React.JSX.Element {
	return (
		<>
			<a
				href="#main-content"
				className="sr-only z-[60] rounded-lg bg-background px-4 py-2 text-sm font-medium shadow-lg focus:fixed focus:top-4 focus:left-4 focus:not-sr-only focus:ring-2 focus:ring-ring"
			>
				Skip to main content
			</a>
			<Suspense fallback={<PageSkeleton />}>
				<Routes>
					<Route path="/test-login" element={<TestLoginPage />} />
					<Route path="/student/*" element={<StudentApp />} />
					<Route path="/admin/*" element={<AdminApp />} />
					<Route path="*" element={<Navigate to="/" replace />} />
				</Routes>
			</Suspense>
		</>
	)
}
