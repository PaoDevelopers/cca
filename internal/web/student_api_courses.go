package web

import (
	"log/slog"
	"net/http"
)

// Students see the whole catalogue, restrictions included, rather than
// only the courses they may take. No course attribute is secret, and a
// visible course with a stated reason ("Year 9 only") explains an
// ineligibility better than a course that silently does not exist.
//
// Which of them the student may actually enter is a separate question
// with a separate endpoint, because it is a different kind of fact:
// the catalogue is the same document for everyone and caches as one,
// while eligibility changes as fast as enrollments do.
func (app *Server) handleStuAPICourses(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPICourses", slog.String("student_id", sui.ID))

	if r.Method != http.MethodGet {
		app.apiMethodNotAllowed(r, w)

		return
	}

	ctx, cancel := readCtx(r.Context())
	defer cancel()

	courses, err := app.queries.GetCourses(ctx)
	if err != nil {
		app.apiDBError(r, w, err)

		return
	}

	app.writeJSON(r, w, courses, slog.String("student_id", sui.ID))
}
