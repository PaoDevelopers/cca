package web

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// Signing in as a student, from the administration area.
//
// Support questions about this system are nearly always about what one
// student is being shown: which courses their window offers them, why
// an enrolment is refused, what their schedule looks like. Reasoning
// about that from the roster is guesswork; looking at their page is
// not. So an administrator may mint a student session for any student
// on the roster and open the student area with it.
//
// It writes the *student* cookie and leaves the administrator's own
// alone. The two areas hold independent sessions in separate cookies —
// that is what makes this safe to do from an open admin tab: the
// administrator does not lose their session, does not have to sign in
// again afterwards, and the student view opens in a second tab beside
// the first. Signing out of the student area, or letting the 72 hours
// run out, ends it; there is nothing else to undo.
//
// The escalation this does not create is worth stating. It hands an
// administrator a session for a role strictly below their own, over an
// endpoint that already requires an administrator session, and it
// cannot mint an admin session for anybody. What it does create is an
// audit question — a placement made in that tab is recorded as the
// student's — which is why it logs both names at info.
//
// The student must exist. A session naming a student the database does
// not have is a cookie that authenticates to an empty catalogue, and
// telling the administrator "no such student" here is more useful than
// letting them find that out in the other tab. GetStudentStatusByID is
// the same existence check the OIDC sign-in path makes, for the same
// reason.
func (app *Server) apiStudentsImpersonate(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	id := r.PathValue("id")

	ctx, cancel := readCtx(r.Context())
	defer cancel()

	if _, err := app.queries.GetStudentStatusByID(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			app.apiMissing(r, w, "student", slog.String("student_id", id))

			return
		}

		app.apiDBError(r, w, err, slog.String("student_id", id))

		return
	}

	token, err := app.sessionKey.encodeSession(roleStudent, id, time.Now().Add(sessionLifetime))
	if err != nil {
		app.apiInternalError(r, w, err, slog.String("student_id", id))

		return
	}

	setCookie(w, studentCookie, token, sessionLifetime)
	app.logInfo(r, logMsgAdminStudentsImpersonate,
		slog.String("admin_username", aui.Username),
		slog.String("student_id", id))
	w.WriteHeader(http.StatusNoContent)
}
