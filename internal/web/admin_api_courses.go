package web

import (
	"log/slog"
	"net/http"

	"github.com/PaoDevelopers/cca/internal/db"
)

// A course is one form and one save. Creating or editing it is one
// call, so the periods, the two restriction lists and the capacity
// arrive together and are judged together: an administrator sees one
// confirm dialog listing everything their edit would break, rather
// than four calls each of which might half-succeed.
//
// The read is one query too. v_courses aggregates the periods and both
// restriction lists, so the list endpoint no longer stitches four
// result sets together in Go.

// courseInput is the request body of both create and update. The id
// comes from the path on update and from the body on create, because
// only create chooses it.
type courseInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CategoryID  string `json:"category_id"`

	Teacher      string `json:"teacher"`
	TeacherEmail string `json:"teacher_email"`
	Location     string `json:"location"`
	Term         string `json:"term"`
	Cost         string `json:"cost"`

	MaxStudents int64 `json:"max_students"`
	InviteOnly  bool  `json:"invite_only"`

	PeriodIDs         []string      `json:"period_ids"`
	AllowedLegalSexes []db.LegalSex `json:"allowed_legal_sexes"`
	AllowedGradeIDs   []string      `json:"allowed_grade_ids"`

	// Violation codes the administrator has confirmed, from a
	// previous attempt that came back 409. Absent on the first try.
	Accept []string `json:"accept"`
}

// The identifier grammars, the non-empty checks and the enum values
// are all the database's, enforced by domains and foreign keys, and a
// rejection comes back as a 400. Restating them here would be a second
// copy to keep in step. What is checked here is only what the database
// cannot see: that the request means one course.
func (in courseInput) validate() string {
	if in.MaxStudents < 0 {
		return "max_students must not be negative"
	}

	return ""
}

func (app *Server) apiCoursesList(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	ctx, cancel := readCtx(r.Context())
	defer cancel()

	courses, err := app.queries.GetCourses(ctx)
	if err != nil {
		app.apiDBError(r, w, err)

		return
	}

	app.writeJSON(r, w, courses)
}

func (app *Server) apiCoursesCreate(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	in, err := decodeBody[courseInput](w, r)
	if err != nil {
		app.apiBadRequest(r, w, "malformed course payload", err)

		return
	}

	if message := in.validate(); message != "" {
		app.apiBadRequest(r, w, message, nil)

		return
	}

	// Creating a course cannot violate anything: it has no enrollees
	// to re-judge, so create_course takes no accept list.
	if err := app.queries.CreateCourse(ctx, db.CreateCourseParams{
		PCourseID:     in.ID,
		PName:         in.Name,
		PDescription:  in.Description,
		PCategoryID:   in.CategoryID,
		PTeacher:      in.Teacher,
		PTeacherEmail: in.TeacherEmail,
		PLocation:     in.Location,
		PTerm:         in.Term,
		PCost:         in.Cost,
		PInviteOnly:   in.InviteOnly,
		PMaxStudents:  in.MaxStudents,
		PPeriodIds:    in.PeriodIDs,
		PLegalSexes:   in.AllowedLegalSexes,
		PGradeIds:     in.AllowedGradeIDs,
	}); err != nil {
		app.apiDBError(r, w, err, slog.String("course_id", in.ID))

		return
	}

	app.logInfo(r, logMsgAdminCoursesCreate, slog.String("admin_username", aui.Username), slog.String("course_id", in.ID))
	app.wsHub.Broadcast(WSMessage("invalidate_courses"))
	w.WriteHeader(http.StatusNoContent)
}

// Editing can break things, so this is the accept path: update_course
// applies the edit, re-judges every enrolled student against the rules
// whose inputs moved, and raises YKV01 unless each violation is named
// in the accept list. The raise undoes the edit, so a refused save
// changes nothing.
func (app *Server) apiCoursesUpdate(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	id := r.PathValue("id")

	in, err := decodeBody[courseInput](w, r)
	if err != nil {
		app.apiBadRequest(r, w, "malformed course payload", err)

		return
	}

	if message := in.validate(); message != "" {
		app.apiBadRequest(r, w, message, nil)

		return
	}

	if err := app.queries.UpdateCourse(ctx, db.UpdateCourseParams{
		PCourseID:     id,
		PName:         in.Name,
		PDescription:  in.Description,
		PCategoryID:   in.CategoryID,
		PTeacher:      in.Teacher,
		PTeacherEmail: in.TeacherEmail,
		PLocation:     in.Location,
		PTerm:         in.Term,
		PCost:         in.Cost,
		PInviteOnly:   in.InviteOnly,
		PMaxStudents:  in.MaxStudents,
		PPeriodIds:    in.PeriodIDs,
		PLegalSexes:   in.AllowedLegalSexes,
		PGradeIds:     in.AllowedGradeIDs,
		PAccept:       in.Accept,
	}); err != nil {
		app.apiDBError(r, w, err, slog.String("course_id", id))

		return
	}

	app.logInfo(r, logMsgAdminCoursesUpdate,
		slog.String("admin_username", aui.Username), slog.String("course_id", id),
		slog.Int("accepted", len(in.Accept)))
	app.wsHub.Broadcast(WSMessage("invalidate_courses"))
	// A rescheduled or re-restricted course changes what its
	// enrollees occupy and what everyone else may join.
	app.wsHub.Broadcast(WSMessage("invalidate_enrollments"))
	w.WriteHeader(http.StatusNoContent)
}

// Renaming an id is not an attribute edit: it cascades through
// enrollments and restrictions, and it invalidates any outstanding
// accept code that names the course, so it is its own operation.
func (app *Server) apiCoursesRename(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	id := r.PathValue("id")

	body, err := decodeBody[struct {
		ID string `json:"id"`
	}](w, r)
	if err != nil {
		app.apiBadRequest(r, w, `expected {"id": "NEWID"}`, err)

		return
	}

	rows, err := app.queries.RenameCourseID(ctx, db.RenameCourseIDParams{
		ID:   id,
		ID_2: body.ID,
	})
	if err != nil {
		app.apiDBError(r, w, err, slog.String("course_id", id))

		return
	}

	if rows == 0 {
		app.apiMissing(r, w, "course", slog.String("course_id", id))

		return
	}

	app.logInfo(r, logMsgAdminCoursesUpdate,
		slog.String("admin_username", aui.Username),
		slog.String("course_id", id), slog.String("new_course_id", body.ID))
	app.wsHub.Broadcast(WSMessage("invalidate_courses"))
	app.wsHub.Broadcast(WSMessage("invalidate_enrollments"))
	w.WriteHeader(http.StatusNoContent)
}

// Deleting is monotone: it can only remove violations, never create
// them, so it takes no accept list. delete_course removes the
// enrollments with it, which is why it is a function rather than the
// foreign key's RESTRICT.
func (app *Server) apiCoursesDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	id := r.PathValue("id")

	if err := app.queries.DeleteCourse(ctx, id); err != nil {
		app.apiDBErrorDeleting(r, w, err, slog.String("course_id", id))

		return
	}

	app.logInfo(r, logMsgAdminCoursesDelete, slog.String("admin_username", aui.Username), slog.String("course_id", id))
	app.wsHub.Broadcast(WSMessage("invalidate_courses"))
	app.wsHub.Broadcast(WSMessage("invalidate_enrollments"))
	w.WriteHeader(http.StatusNoContent)
}
