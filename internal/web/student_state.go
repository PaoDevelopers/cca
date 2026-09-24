package web

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/PaoDevelopers/cca/internal/db"
)

// Why a student's own write pushes its result instead of asking the
// pages to read it.
//
// Everything a student's write changes on their own page — the courses
// they hold, what they may take next, their standing — used to reach
// their other pages as invalidate_enrollments, after which each page
// read all three back: three requests per page, eligibility among them,
// and the writing page read two of them again as well. With a student
// on three tabs that was about nine requests per enrollment, and in a
// simulated rush those follow-up reads were most of the database's work.
//
// The server has to read that state once anyway, to answer the write.
// Reading it once more, whole, and handing it to every one of the
// student's pages costs one eligibility query per write, however many
// pages they have open.
//
// Administrators' pages still get invalidate_enrollments: they show
// every student, and this state is one student's.

// studentState is the payload of a student_state frame.
type studentState struct {
	Enrollments []db.VEnrollment `json:"enrollments"`
	Eligibility eligibility      `json:"eligibility"`
	User        studentInfo      `json:"user"`
}

func (app *Server) loadStudentState(ctx context.Context, studentID string) (studentState, error) {
	enrollments, err := app.queries.GetEnrollmentsByStudent(ctx, studentID)
	if err != nil {
		return studentState{}, fmt.Errorf("enrollments: %w", err)
	}

	elig, err := app.eligibility.get(ctx, studentID, app.loadEligibility)
	if err != nil {
		return studentState{}, err
	}

	user, err := app.loadStudentInfo(ctx, studentID)
	if err != nil {
		return studentState{}, err
	}

	return studentState{Enrollments: enrollments, Eligibility: elig, User: user}, nil
}

// respondWithState answers a student's write with their enrollments and
// pushes their whole new state to all of their pages.
//
// The read is detached from the request: the student who closed the
// tab as they clicked still has other pages to tell. If it fails, the
// write has still happened, so the pages are told to read for
// themselves, as they were before this existed.
func (app *Server) respondWithState(w http.ResponseWriter, r *http.Request, studentID string) {
	gen := app.wsHub.NextStateGeneration()

	ctx, cancel := readCtx(context.WithoutCancel(r.Context()))
	defer cancel()

	state, err := app.loadStudentState(ctx, studentID)
	if err != nil {
		app.wsHub.BroadcastToStudents([]string{studentID}, WSMessage("invalidate_enrollments"))
		app.apiDBError(r, w, err, slog.String("student_id", studentID))

		return
	}

	frame, err := json.Marshal(state)
	if err != nil {
		app.logError(r, logMsgHTTPResponseEncodeError, slog.Any("error", err))
		app.wsHub.BroadcastToStudents([]string{studentID}, WSMessage("invalidate_enrollments"))
	} else {
		app.wsHub.PushStudentState(studentID, gen, WSMessage("student_state,"+string(frame)))
	}

	app.writeJSON(r, w, state.Enrollments, slog.String("student_id", studentID))
}
