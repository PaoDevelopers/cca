package web

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"time"
)

// maxJSONBody bounds every JSON write. The largest legitimate body is
// a bulk assignment's two id arrays, which is orders of magnitude
// under this; the CSV imports have their own, larger limit.
const maxJSONBody = 1 << 20

// bodyReadTimeout bounds how long a request may take to deliver its
// body, as opposed to how large the body may be.
//
// The server cannot set ReadTimeout globally: the event websockets are
// hijacked connections that stay open for hours, and a read deadline
// would kill them. So the limit is applied per handler, on exactly the
// routes that read a body — which is also where the risk is. Without
// it a client can send one byte a minute and hold a connection and a
// goroutine for as long as it likes; MaxBytesReader bounds the size
// and says nothing about the time.
const bodyReadTimeout = 30 * time.Second

// boundBodyRead gives the request a deadline for delivering its body.
// A connection that does not support deadlines (a test recorder, a
// wrapper) simply does not get one.
func boundBodyRead(w http.ResponseWriter, timeout time.Duration) {
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(timeout))
}

// decodeBody needs the ResponseWriter because MaxBytesReader uses it to
// stop the connection once a body runs over, rather than reading the
// rest of it into nothing.
func decodeBody[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var v T

	boundBodyRead(w, bodyReadTimeout)

	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)

	if err := json.UnmarshalRead(r.Body, &v, json.RejectUnknownMembers(true)); err != nil {
		return v, fmt.Errorf("decode request body: %w", err)
	}

	return v, nil
}

func (app *Server) registerAdminAPI(mux *http.ServeMux, routes *apiRoutes) {
	handle := func(pattern string, name string, handler func(http.ResponseWriter, *http.Request, *UserInfoAdmin)) {
		routes.record(pattern)
		mux.HandleFunc(pattern, app.adminAPI(name, handler))
	}

	handle("GET /admin/api/events", "apiEvents", app.apiEvents)

	categories := app.categories()
	handle("GET /admin/api/categories", "apiCategoriesList", app.apiCategoriesList)
	handle("POST /admin/api/categories", "apiCategoriesCreate", app.createNamed(categories))
	handle("PUT /admin/api/categories/{id}", "apiCategoriesRename", app.renameNamed(categories))
	handle("DELETE /admin/api/categories/{id}", "apiCategoriesDelete", app.deleteNamed(categories))

	periods := app.periods()
	handle("GET /admin/api/periods", "apiPeriodsList", app.apiPeriodsList)
	handle("POST /admin/api/periods", "apiPeriodsCreate", app.createNamed(periods))
	// Before the {id} patterns: "order" is a literal segment, and
	// ServeMux prefers it, but stating the intent beats relying on it.
	handle("POST /admin/api/periods/order", "apiPeriodsOrder", app.apiPeriodsOrder)
	handle("PUT /admin/api/periods/{id}", "apiPeriodsRename", app.renameNamed(periods))
	handle("DELETE /admin/api/periods/{id}", "apiPeriodsDelete", app.deleteNamed(periods))

	handle("GET /admin/api/grades", "apiGradesList", app.apiGradesList)
	handle("POST /admin/api/grades", "apiGradesCreate", app.apiGradesCreate)
	handle("POST /admin/api/grades/order", "apiGradesOrder", app.apiGradesOrder)
	handle("POST /admin/api/grades/close", "apiGradesCloseAll", app.apiGradesCloseAll)
	handle("PUT /admin/api/grades/{id}", "apiGradesUpdate", app.apiGradesUpdate)
	handle("PUT /admin/api/grades/{id}/window", "apiGradesWindow", app.apiGradesWindow)
	handle("POST /admin/api/grades/{id}/window/open", "apiGradesOpenNow", app.apiGradesOpenNow)
	handle("POST /admin/api/grades/{id}/window/close", "apiGradesCloseNow", app.apiGradesCloseNow)
	handle("PUT /admin/api/grades/{id}/budget", "apiGradesBudget", app.apiGradesBudget)
	handle("PUT /admin/api/grades/{id}/requirements", "apiGradesRequirements", app.apiGradesRequirements)
	handle("DELETE /admin/api/grades/{id}", "apiGradesDelete", app.apiGradesDelete)

	handle("GET /admin/api/courses", "apiCoursesList", app.apiCoursesList)
	handle("POST /admin/api/courses", "apiCoursesCreate", app.apiCoursesCreate)
	handle("PUT /admin/api/courses/{id}", "apiCoursesUpdate", app.apiCoursesUpdate)
	handle("PUT /admin/api/courses/{id}/id", "apiCoursesRename", app.apiCoursesRename)
	handle("DELETE /admin/api/courses/{id}", "apiCoursesDelete", app.apiCoursesDelete)

	handle("GET /admin/api/students", "apiStudentsList", app.apiStudentsList)
	handle("GET /admin/api/students/status", "apiStudentsStatus", app.apiStudentsStatus)
	// One endpoint for the roster import and the single-student edit
	// alike: both mean "make these students be so", and upsert is
	// idempotent, so re-sending either is a no-op.
	handle("PUT /admin/api/students", "apiStudentsUpsert", app.apiStudentsUpsert)
	handle("DELETE /admin/api/students/{id}", "apiStudentsDelete", app.apiStudentsDelete)
	// Mints a student session for the administrator's own browser, in
	// the student cookie, so the admin session beside it is untouched.
	handle("POST /admin/api/students/{id}/session", "apiStudentsImpersonate", app.apiStudentsImpersonate)

	handle("GET /admin/api/enrollments", "apiEnrollmentsList", app.apiEnrollmentsList)
	handle("POST /admin/api/enrollments", "apiEnrollmentsPlace", app.apiEnrollmentsPlace)
	handle("PUT /admin/api/enrollments/policy", "apiEnrollmentsPolicy", app.apiEnrollmentsPolicy)
	handle("DELETE /admin/api/enrollments", "apiEnrollmentsRemove", app.apiEnrollmentsRemove)

	handle("POST /admin/api/courses/import", "handleAdmCoursesImport", app.handleAdmCoursesImport)
	handle("POST /admin/api/students/import", "handleAdmStudentsImport", app.handleAdmStudentsImport)
	handle("POST /admin/api/enrollments/import", "handleAdmEnrollmentsImport", app.handleAdmEnrollmentsImport)
	handle("GET /admin/api/enrollments/export", "handleAdmEnrollmentsExport", app.handleAdmEnrollmentsExport)
	handle("GET /admin/api/students/export", "handleAdmStudentsExport", app.handleAdmStudentsExport)
	handle("GET /admin/api/courses/export", "handleAdmCoursesExport", app.handleAdmCoursesExport)

	handle("DELETE /admin/api/data/{section}", "apiDataClear", app.apiDataClear)
}
