package web

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/PaoDevelopers/cca/internal/db"
)

// A student has exactly three writes, and that is the whole of what a
// student can mean: enroll, drop, and swap. Declining an invitation is
// not a fourth — an invitation is an enrollment the student may drop,
// so declining is dropping.
//
// Swap is not sugar for drop-then-add. "I want this instead of that"
// is one intention, and judging the new course with the old ones
// disregarded is the only way to swap between two courses that clash.
//
// None of the three takes an accept list. Students accept nothing: the
// negotiable rules are negotiable by administrators, whose input is
// the sanction that makes an exception an exception.
//
// The three differ only in which function they call and what they
// name, so the decode, the write, the notification and the reply are
// one path parameterized by a studentWrite.

// studentWrite is one of the three, resolved from the method.
type studentWrite struct {
	operation  string
	logMessage string

	// call performs it; body has already been decoded.
	call func(ctx context.Context, q *db.Queries, studentID string, body selfEnrollmentBody) error

	// coursesTouched are the courses whose enrollee count moved.
	coursesTouched func(body selfEnrollmentBody) []string
}

// selfEnrollmentBody is the request body of all three. Replacing is
// meaningful only to a swap, and is absent from the others.
type selfEnrollmentBody struct {
	CourseID  string   `json:"course_id"`
	Replacing []string `json:"replacing"`
}

// maxReplacing bounds a swap's replacing list.
//
// The body limit alone is not a bound on work: a megabyte of repeated
// course ids is a hundred and seventy thousand array elements, and the
// locking and existence checks walk them. A student cannot hold more
// courses than there are periods in a week, so anything approaching
// this is not a swap.
const maxReplacing = 64

func (b selfEnrollmentBody) validate() string {
	if b.CourseID == "" {
		return "course_id must not be empty"
	}

	if len(b.Replacing) > maxReplacing {
		return "a swap cannot replace that many courses"
	}

	return ""
}

//nolint:gochecknoglobals // a routing table, immutable after init
var studentWrites = map[string]studentWrite{
	http.MethodPut: {
		operation:  "self_enroll",
		logMessage: logMsgStudentEnroll,
		call: func(ctx context.Context, q *db.Queries, studentID string, body selfEnrollmentBody) error {
			return q.SelfEnroll(ctx, db.SelfEnrollParams{
				PStudentID: studentID, PCourseID: body.CourseID,
			})
		},
		coursesTouched: func(body selfEnrollmentBody) []string {
			return []string{body.CourseID}
		},
	},
	http.MethodDelete: {
		operation:  "self_drop",
		logMessage: logMsgStudentDrop,
		call: func(ctx context.Context, q *db.Queries, studentID string, body selfEnrollmentBody) error {
			return q.SelfDrop(ctx, db.SelfDropParams{
				PStudentID: studentID, PCourseID: body.CourseID,
			})
		},
		coursesTouched: func(body selfEnrollmentBody) []string {
			return []string{body.CourseID}
		},
	},
	http.MethodPost: {
		operation:  "self_swap",
		logMessage: logMsgStudentSwap,
		call: func(ctx context.Context, q *db.Queries, studentID string, body selfEnrollmentBody) error {
			return q.SelfSwap(ctx, db.SelfSwapParams{
				PStudentID:    studentID,
				POldCourseIds: body.Replacing,
				PCourseID:     body.CourseID,
			})
		},
		coursesTouched: func(body selfEnrollmentBody) []string {
			// A swap moves the count of everything it drops as well
			// as of what it takes.
			return append(append([]string{}, body.Replacing...), body.CourseID)
		},
	},
}

func (app *Server) handleStuAPIMyEnrollments(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPIMyEnrollments", slog.String("student_id", sui.ID))

	// Students are the untrusted many, so bound their bodies too.
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)

	if r.Method == http.MethodGet {
		app.respondWithEnrollments(w, r, sui.ID)

		return
	}

	write, ok := studentWrites[r.Method]
	if !ok {
		app.apiMethodNotAllowed(r, w)

		return
	}

	body, err := decodeBody[selfEnrollmentBody](w, r)
	if err != nil {
		app.apiBadRequest(r, w, `expected {"course_id": "..."}`, err,
			slog.String("operation", write.operation), slog.String("student_id", sui.ID))

		return
	}

	if message := body.validate(); message != "" {
		app.apiBadRequest(r, w, message, nil,
			slog.String("operation", write.operation), slog.String("student_id", sui.ID))

		return
	}

	// Detached from the request: a student who closes the tab as they
	// click still has their change completed and, more to the point,
	// announced to everyone else waiting on the seat count.
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	if err := write.call(ctx, app.queries, sui.ID, body); err != nil {
		app.apiDBError(r, w, err,
			slog.String("operation", write.operation),
			slog.String("student_id", sui.ID), slog.String("course_id", body.CourseID))

		return
	}

	app.logInfo(r, write.logMessage,
		slog.String("student_id", sui.ID), slog.String("course_id", body.CourseID),
		slog.Any("replacing", body.Replacing))
	app.wsHub.BroadcastToStudentsAndAdmins([]string{sui.ID}, WSMessage("invalidate_enrollments"))
	app.broadcastCourseCounts(write.coursesTouched(body))

	// Every write answers with the resulting enrollment set, so a
	// client never has to guess what its change did and never shows a
	// state the server did not confirm.
	app.respondWithEnrollments(w, r, sui.ID)
}

func (app *Server) respondWithEnrollments(w http.ResponseWriter, r *http.Request, studentID string) {
	ctx, cancel := readCtx(r.Context())
	defer cancel()

	enrollments, err := app.queries.GetEnrollmentsByStudent(ctx, studentID)
	if err != nil {
		app.apiDBError(r, w, err, slog.String("student_id", studentID))

		return
	}

	app.writeJSON(r, w, enrollments, slog.String("student_id", studentID))
}
