package httpapi

import (
	"context"
	"net/http"
)

// StartBackground starts the realtime hub and grade access scheduler.
func (app *App) StartBackground(ctx context.Context) {
	go app.wsHub.Run()
	go app.runGradeSelectionScheduler(ctx)
}

// Handler returns the complete application route tree.
func (app *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", app.handleHealthz)
	mux.HandleFunc("/{$}", app.handleIndex)
	mux.HandleFunc("/auth", app.handleAuth)
	mux.HandleFunc("/dp42ik", app.handleDP42IK)
	mux.Handle("/assets/", frontendAssetsHandler())

	mux.HandleFunc("/api/v1/session", app.handleAPISession)
	mux.HandleFunc("/api/v1/test-auth", app.handleAPITestAuth)
	mux.HandleFunc("/api/v1/student/bootstrap", app.apiStudentOnly("handleAPIStudentBootstrap", app.handleAPIStudentBootstrap))
	mux.HandleFunc("/api/v1/student/courses", app.apiStudentOnly("handleAPIStudentCourses", app.handleAPIStudentCourses))
	mux.HandleFunc("/api/v1/student/periods", app.apiStudentOnly("handleAPIStudentPeriods", app.handleAPIStudentPeriods))
	mux.HandleFunc("/api/v1/student/grades", app.apiStudentOnly("handleAPIStudentGrades", app.handleAPIStudentGrades))
	mux.HandleFunc("/api/v1/student/selections", app.apiStudentOnly("handleAPIStudentSelections", app.handleAPIStudentSelections))
	mux.HandleFunc("/api/v1/student/events", app.apiStudentOnly("handleStuAPIEvents", app.handleStuAPIEvents))
	mux.HandleFunc("/api/v1/admin/dashboard", app.apiAdminOnly("handleAPIAdminDashboard", app.handleAPIAdminDashboard))
	mux.HandleFunc("/api/v1/admin/bootstrap", app.apiAdminOnly("handleAPIAdminBootstrap", app.handleAPIAdminBootstrap))
	mux.HandleFunc("/api/v1/admin/categories", app.apiAdminOnly("handleAPIAdminCategories", app.handleAPIAdminCategories))
	mux.HandleFunc("/api/v1/admin/categories/{id}", app.apiAdminOnly("handleAPIAdminCategory", app.handleAPIAdminCategory))
	mux.HandleFunc("/api/v1/admin/periods", app.apiAdminOnly("handleAPIAdminPeriods", app.handleAPIAdminPeriods))
	mux.HandleFunc("/api/v1/admin/grades", app.apiAdminOnly("handleAPIAdminGrades", app.handleAPIAdminGrades))
	mux.HandleFunc("/api/v1/admin/grades/{grade}", app.apiAdminOnly("handleAPIAdminGrade", app.handleAPIAdminGrade))
	mux.HandleFunc("/api/v1/admin/grade-access", app.apiAdminOnly("handleAPIAdminGradeAccess", app.handleAPIAdminGradeAccess))
	mux.HandleFunc("/api/v1/admin/grade-schedules", app.apiAdminOnly("handleAPIAdminGradeSchedules", app.handleAPIAdminGradeSchedules))
	mux.HandleFunc("/api/v1/admin/grade-schedules/{batch_id}", app.apiAdminOnly("handleAPIAdminGradeSchedule", app.handleAPIAdminGradeSchedule))
	mux.HandleFunc("/api/v1/admin/grades/{grade}/requirement-groups", app.apiAdminOnly("handleAPIAdminRequirementGroups", app.handleAPIAdminRequirementGroups))
	mux.HandleFunc("/api/v1/admin/grades/{grade}/requirement-groups/{id}", app.apiAdminOnly("handleAPIAdminRequirementGroup", app.handleAPIAdminRequirementGroup))
	mux.HandleFunc("/api/v1/admin/courses", app.apiAdminOnly("handleAPIAdminCourses", app.handleAPIAdminCourses))
	mux.HandleFunc("/api/v1/admin/courses/{id}", app.apiAdminOnly("handleAPIAdminCourse", app.handleAPIAdminCourse))
	mux.HandleFunc("/api/v1/admin/students", app.apiAdminOnly("handleAPIAdminStudents", app.handleAPIAdminStudents))
	mux.HandleFunc("/api/v1/admin/students/{id}", app.apiAdminOnly("handleAPIAdminStudent", app.handleAPIAdminStudent))
	mux.HandleFunc("/api/v1/admin/selections", app.apiAdminOnly("handleAPIAdminSelections", app.handleAPIAdminSelections))
	mux.HandleFunc("/api/v1/admin/selections/{student_id}/{course_id}", app.apiAdminOnly("handleAPIAdminSelection", app.handleAPIAdminSelection))
	mux.HandleFunc("/api/v1/admin/notifications", app.apiAdminOnly("handleAPIAdminNotifications", app.handleAPIAdminNotifications))
	mux.HandleFunc("/api/v1/admin/reset", app.apiAdminOnly("handleAPIAdminReset", app.handleAPIAdminReset))

	mux.HandleFunc("/admin/data/examples/{kind}", app.apiAdminOnly("handleAdmDataExample", app.handleAdmDataExample))
	mux.HandleFunc("/admin/courses/import", app.apiAdminOnly("handleAdmCoursesImport", app.handleAdmCoursesImport))
	mux.HandleFunc("/admin/students/import", app.apiAdminOnly("handleAdmStudentsImport", app.handleAdmStudentsImport))
	mux.HandleFunc("/admin/selections/export", app.apiAdminOnly("handleAdmSelectionsExport", app.handleAdmSelectionsExport))
	mux.HandleFunc("/admin/selections/import", app.apiAdminOnly("handleAdmSelectionsImport", app.handleAdmSelectionsImport))
	mux.HandleFunc("/student", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/student/", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/student/{path...}", app.studentOnlyPlain("studentFrontend", serveFrontendIndex))
	mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/admin/{path...}", app.adminOnlyPlain("adminFrontend", serveFrontendIndex))
	mux.HandleFunc("/test-login", app.handleTestAuthFrontend)
	mux.HandleFunc("/test-login/{path...}", app.handleTestAuthFrontend)
	return mux
}
