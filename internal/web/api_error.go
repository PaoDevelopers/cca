package web

import (
	"context"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
)

// The single error shape for every API response, including the
// endpoints that serve CSV on success:
//
//	{"error": {"code": "clash", "message": "..."}}
//
// Codes are a closed set and are what callers branch on. The message
// is prose for display.
//
// Two codes carry a machine payload beside the message, because the
// frontend has to do something with it rather than only show it:
//
//	violations  the negotiable rules a write would break, each with
//	            the code that accepts it; the confirm dialog lists
//	            these and sends back the codes the user agreed to
//	malformed   the elements of a batch that could not be read at
//	            all, each with its row and the column it was rejected
//	            on, so a bad spreadsheet is fixed in one pass rather
//	            than one row per upload
const (
	codeBadRequest       = "bad_request"
	codeUnauthenticated  = "unauthenticated"
	codeForbidden        = "forbidden"
	codeNotFound         = "not_found"
	codeMethodNotAllowed = "method_not_allowed"
	codeConflict         = "conflict"
	codeInternal         = "internal"

	// Gates. Absolute, never negotiable, and distinct from each
	// other because each has a different remedy: wait, ask an
	// administrator, or ask an administrator to release you.
	codeWindowClosed = "window_closed"
	codeInviteOnly   = "invite_only"
	codeNotDroppable = "not_droppable"

	// Negotiable, and the batch counterpart of a bad request.
	codeViolations = "violations"
	codeMalformed  = "malformed"

	// The database could not answer. Distinct from codeInternal
	// because the remedy is different: wait, rather than report.
	codeUnavailable = "unavailable"
)

// statusClientClosedRequest is nginx's 499. Not in net/http because it
// is not in the RFC, but it is what every log aggregator already knows
// how to read, and it keeps "the caller left" out of the 5xx bucket.
const statusClientClosedRequest = 499

// A violation as the database reports it. Mirrors the JSON objects in
// a YKV01 DETAIL exactly; see internal/db/schemas/0012 for the codes'
// grammar. Code is what an accept names, and the only field the client
// must send back.
type apiViolation struct {
	StudentID     *string `json:"student_id"`
	Rule          string  `json:"rule"`
	Code          string  `json:"code"`
	OtherCourseID *string `json:"other_course_id"`
	PeriodID      *string `json:"period_id"`
	Detail        string  `json:"detail"`
}

// A batch element that could not be read. Mirrors a YKD01 DETAIL,
// except that Message is rewritten before it goes out: what the
// database puts there is SQLERRM, which names domains and constraints.
type apiMalformed struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	SQLState string `json:"sqlstate"`
	Message  string `json:"message"`

	// The import column the value came from, in the header's own
	// spelling, so the message can say which cell to go and fix. It
	// is sent: unlike the two fields below it names the spreadsheet
	// rather than the schema. '' where the write function could not
	// attribute the failure to one column.
	Field string `json:"field"`

	// The machine-readable half, used to choose the message above and
	// to sharpen Field. Never sent: a client has no use for them and
	// they name schema internals. json/v2 omits them from output
	// because of the tags, and the decoder fills them from the
	// database's payload.
	Constraint string `json:"constraint,omitzero"`
	Column     string `json:"column,omitzero"`
}

type apiErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`

	// Present only for their own codes; omitted entirely otherwise,
	// so a client can test for the field as well as the code.
	Violations []apiViolation `json:"violations,omitempty"`
	Malformed  []apiMalformed `json:"malformed,omitempty"`
}

type apiErrorBody struct {
	Error apiErrorDetail `json:"error"`
}

// err is logged, never sent to the client unless also passed as message.
func (app *Server) apiError(r *http.Request, w http.ResponseWriter, status int, code string, message string, err error, extra ...slog.Attr) {
	app.apiErrorDetail(r, w, status, apiErrorDetail{
		Code: code, Message: message,
		Violations: nil, Malformed: nil,
	}, err, extra...)
}

func (app *Server) apiErrorDetail(r *http.Request, w http.ResponseWriter, status int, detail apiErrorDetail, err error, extra ...slog.Attr) {
	apiHeaders(w)

	attrs := []slog.Attr{
		slog.Int("status", status),
		slog.String("code", detail.Code),
		slog.String("message", detail.Message),
	}
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}

	if n := len(detail.Violations); n > 0 {
		attrs = append(attrs, slog.Int("violation_count", n))
	}

	if n := len(detail.Malformed); n > 0 {
		attrs = append(attrs, slog.Int("malformed_count", n))
	}

	attrs = append(attrs, extra...)

	// Error level means "somebody has to look at this". Most 5xx
	// answers qualify; a deliberate 503 does not — it is the system
	// correctly reporting that it cannot serve right now, the caller
	// is being told to retry, and the outage is already being
	// announced once by the code that detected it. Logging it here as
	// well turns one outage into two error lines per in-flight
	// request.
	switch {
	case status == http.StatusServiceUnavailable:
		app.logWarn(r, logMsgAPIResponseError, attrs...)
	case status >= http.StatusInternalServerError:
		app.logError(r, logMsgAPIResponseError, attrs...)
	default:
		app.logWarn(r, logMsgAPIResponseError, attrs...)
	}

	w.WriteHeader(status)

	if err := json.MarshalWrite(w, apiErrorBody{Error: detail}); err != nil {
		app.logError(r, logMsgAPIResponseEncodeError, slog.Any("error", err))
	}
}

// apiMissing reports an update or delete that matched no row. Saying
// 204 instead would tell an administrator that an edit landed on
// something somebody else had just removed.
func (app *Server) apiMissing(r *http.Request, w http.ResponseWriter, what string, extra ...slog.Attr) {
	app.apiError(r, w, http.StatusNotFound, codeNotFound, "no such "+what, nil, extra...)
}

func (app *Server) apiBadRequest(r *http.Request, w http.ResponseWriter, message string, err error, extra ...slog.Attr) {
	app.apiError(r, w, http.StatusBadRequest, codeBadRequest, message, err, extra...)
}

// apiInternalError answers a failure nobody anticipated.
//
// The message is fixed. Whatever went wrong, its text was written for
// whoever operates this, not for a student — and the ways it can be
// worse than useless are not hypothetical: pgx's connection error
// spells out the database user and database name, PostgreSQL's own
// messages name relations and constraints, and a failed check carries
// the whole offending row. The error itself goes to the log, where the
// person who can act on it will find it.
func (app *Server) apiInternalError(r *http.Request, w http.ResponseWriter, err error, extra ...slog.Attr) {
	app.apiError(r, w, http.StatusInternalServerError, codeInternal,
		"Something went wrong on the server. Please try again; if it "+
			"keeps happening, tell an administrator.", err, extra...)
}

func (app *Server) apiMethodNotAllowed(r *http.Request, w http.ResponseWriter, extra ...slog.Attr) {
	app.apiError(r, w, http.StatusMethodNotAllowed, codeMethodNotAllowed, "Method not allowed", nil, extra...)
}

// What the database's own messages say, translated for the person
// reading them.
//
// Our own raises (the YK class) are written for a human and are passed
// through as they are. PostgreSQL's built-in rejections are not: their
// messages name constraints, domains and relations — "value for domain
// entity_id violates check constraint entity_id_check" — and their
// DETAIL for a failed check contains the entire failing row, which for
// a student row is personal data. None of that belongs in front of a
// CCA administrator, and the DETAIL belongs nowhere but the log.
//
// So every built-in SQLSTATE gets a message written here. Where the
// constraint is one of the identifier domains, the message says what
// the format actually is, because that is the one thing the person
// needs in order to fix it.
//
//nolint:gochecknoglobals
var domainMessages = map[string]string{
	"entity_id_check": "An ID must be 1 to 16 characters, using only " +
		"capital letters, digits, hyphen and underscore.",
	"localpart_check": "A student ID must be the localpart of their school " +
		"email address, in lowercase — for example s22537.",
	"trimmed_text_check": "This must not be empty, and must not begin or " +
		"end with a space.",
	"trimmed_text_opt_check": "This must not begin or end with a space.",
	"count_value_check": "This must be a whole number between 0 and " +
		"1000000.",
}

// Database column names to the import header names they are filled
// from. Consulted only for a not-null violation, which is the one
// rejection that names its column and the one the write functions
// cannot attribute themselves: every other failure is a cast, and a
// cast is attributed by the statement it happens in.
//
// grade_id is a student's grade here. A course's allowed_grades is
// also grade_id, but only a JSON null could reach that column's NOT
// NULL, and its own statement has already named the column correctly
// by then.
//
//nolint:gochecknoglobals
var columnFields = map[string]string{
	"id":            "id",
	"name":          "name",
	"description":   "description",
	"max_students":  "max_students",
	"invite_only":   "invite_only",
	"teacher":       "teacher",
	"teacher_email": "teacher_email",
	"location":      "location",
	"term":          "term",
	"cost":          "cost",
	"category_id":   "category",
	"grade_id":      "grade",
	"legal_sex":     "legal_sex",
	"period_id":     "periods",
}

// elementField says which import column the element was rejected on.
//
// The write function names it, because a domain rejection does not:
// PostgreSQL raises it while casting a value, before the value has
// reached a column at all, so the error carries the domain and the
// constraint and nothing about where the value was going. The one
// exception is a not-null violation, which does name its column and
// is the one thing the statement boundary cannot pin down.
func elementField(element apiMalformed) string {
	if element.SQLState == "23502" {
		if field, named := columnFields[element.Column]; named {
			return field
		}
	}

	return element.Field
}

// elementMessage says what is wrong with one batch element, in the
// terms the person editing the spreadsheet works in.
func elementMessage(element apiMalformed) string {
	if message, named := domainMessages[element.Constraint]; named {
		return message
	}

	switch element.SQLState {
	case "23503":
		return "This names a category, grade or period that does not exist."
	case "22P02":
		return "A value here is not one of the allowed ones."
	case "23502":
		return "A required value is missing."
	}

	return "This row could not be read."
}

// dbErrorDetail classifies an error raised by a database write
// function. ok is false when the error is not a recognized rejection,
// which is the caller's cue to treat it as internal: an unclassified
// SQLSTATE is a defect here, not a message for a user.
//
// deleting says whether the caller was removing something. It is the
// one thing the SQLSTATE cannot express: a foreign key violation means
// "still referred to" when deleting and "refers to something absent"
// otherwise, and PostgreSQL distinguishes them only in prose. The
// handler always knows which it meant, so it says.
//
// The YK class is ours; see the error vocabulary at the head of
// internal/db/schemas/0013_enrollment_writes.sql.
func dbErrorDetail(err error, deleting bool) (status int, detail apiErrorDetail, ok bool) {
	//exhaustruct:ignore
	detail = apiErrorDetail{}

	pgErr, fromPostgres := errors.AsType[*pgconn.PgError](err)
	if !fromPostgres {
		return 0, detail, false
	}

	detail.Message = pgErr.Message

	switch pgErr.Code {
	case "YKV01":
		// The payload is the point: without it the client cannot
		// offer to accept anything. A DETAIL that will not decode is
		// a defect in the schema, so it becomes an internal error
		// rather than a dialog with nothing in it.
		var vs []apiViolation
		if json.Unmarshal([]byte(pgErr.Detail), &vs) != nil || len(vs) == 0 {
			return 0, detail, false
		}

		detail.Code = codeViolations
		detail.Violations = vs

		return http.StatusConflict, detail, true

	case "YKD01":
		var ms []apiMalformed
		if json.Unmarshal([]byte(pgErr.Detail), &ms) != nil || len(ms) == 0 {
			return 0, detail, false
		}

		for i := range ms {
			ms[i].Message = elementMessage(ms[i])
			ms[i].Field = elementField(ms[i])
			// Chosen the message and the column; the constraint and
			// the database's own column name are schema internals and
			// go no further.
			ms[i].Constraint = ""
			ms[i].Column = ""
		}

		detail.Code = codeMalformed
		detail.Malformed = ms

		return http.StatusBadRequest, detail, true

	// The three gates are raised only by the self_* functions, so the
	// person reading them is always a student. Their database
	// messages are written for whoever reads the log — they name the
	// student to themselves ("enrollment window is closed for student
	// s22537") and name courses by internal id — so each gets prose
	// written for the person who will actually see it. The database's
	// wording stays in the log, where it was aimed.
	case "YKG01":
		detail.Code = codeWindowClosed
		detail.Message = "Enrollment is not open for your year group " +
			"at the moment."

		return http.StatusForbidden, detail, true

	case "YKG02":
		detail.Code = codeInviteOnly
		detail.Message = "This activity is invitation only. Ask the CCA " +
			"office if you think you should be in it."

		return http.StatusForbidden, detail, true

	case "YKG03":
		// An administrator placed them and did not leave the door
		// open. Ordinary and expected — a student can click Drop on
		// such a row — so it must not be an internal error.
		detail.Code = codeNotDroppable
		detail.Message = "This activity was arranged for you, so it " +
			"cannot be dropped here. Ask the CCA office."

		return http.StatusForbidden, detail, true

	case "P0002": // no_data_found
		detail.Code = codeNotFound

		return http.StatusNotFound, detail, true

	case "23505": // unique_violation
		detail.Code = codeConflict
		detail.Message = "That already exists."

		return http.StatusConflict, detail, true

	case "23503": // foreign_key_violation
		detail.Code = codeConflict
		if deleting {
			// Names the usual culprit, because it is the usual
			// culprit: enrollments refer to courses, students and
			// grades alike, and clearing them is the step that gets
			// skipped.
			detail.Message = "This is still in use, so it cannot be " +
				"deleted. Clear the enrollments first, or remove " +
				"whatever else still refers to it."
		} else {
			detail.Message = "This refers to something that does not exist."
		}

		return http.StatusConflict, detail, true

	case "22023":
		// invalid_parameter_value. Nothing built into PostgreSQL
		// raises this here: the domains reject with 23514 and the
		// enums with 22P02. Every 22023 in this system comes from a
		// RAISE in the schema, written for the person who will read
		// it — "the order must name every period exactly once",
		// "requirement must name at least one category" — so it is
		// passed through like the YK class rather than replaced.
		//
		// It used to be bucketed with the built-ins below, which
		// overwrote all fifteen of them with "That value is not valid
		// here." The refusal happened and the administrator was told
		// nothing they could act on.
		detail.Code = codeBadRequest

		return http.StatusBadRequest, detail, true

	case "23514", "22P02", "22021":
		// check_violation, invalid_text_representation,
		// character_not_in_repertoire: the identifier domains, the
		// enums, and the encoding reject ill-formed input this way, so
		// these are the user's typo — or their fuzzer — not the
		// server's fault.
		detail.Code = codeBadRequest
		if message, named := domainMessages[pgErr.ConstraintName]; named {
			detail.Message = message
		} else {
			detail.Message = "That value is not valid here."
		}

		return http.StatusBadRequest, detail, true

	case "40P01", "40001": // deadlock_detected, serialization_failure
		// Transient: nothing was wrong with the request, two writers
		// simply collided. Reporting it as a server error would send
		// the administrator looking for a fault that is not there.
		detail.Code = codeConflict
		detail.Message = "Another administrator was writing at the same time; try again"

		return http.StatusConflict, detail, true
	}

	return 0, detail, false
}

func (app *Server) apiDBError(r *http.Request, w http.ResponseWriter, err error, extra ...slog.Attr) {
	app.dbError(r, w, err, false, extra...)
}

// apiDBErrorDeleting is apiDBError for a handler that was removing
// something, so that a foreign key violation reads as "still in use"
// rather than "refers to something absent".
func (app *Server) apiDBErrorDeleting(r *http.Request, w http.ResponseWriter, err error, extra ...slog.Attr) {
	app.dbError(r, w, err, true, extra...)
}

func (app *Server) dbError(r *http.Request, w http.ResponseWriter, err error, deleting bool, extra ...slog.Attr) {
	// A browser that navigated away mid-request cancels its context,
	// the query fails with it, and there is nobody left to answer.
	// Reporting that as a server fault fills the log with errors for
	// the most ordinary thing a user does — and hides the real ones.
	// Our own write timeout is a different matter and stays an error.
	if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
		app.logInfo(r, logMsgClientGone, append(extra, slog.Any("error", err))...)
		// The conventional code for it. Nothing will read this.
		w.WriteHeader(statusClientClosedRequest)

		return
	}

	// The database being away, or too busy to answer inside the
	// ceiling, is not a fault in this request and not a fault in the
	// code. Telling the user to fetch an administrator is wrong
	// advice — the right advice is to try again — and logging it at
	// error level turns one outage into a page per in-flight request,
	// burying the errors that do need someone.
	if transient, why := transientFailure(err); transient {
		// The cause is named once, here, where it is known. The
		// response itself logs only that a 503 went out.
		app.logWarn(r, logMsgDatabaseUnavailable,
			append(extra, slog.Any("error", err), slog.String("cause", why))...)
		app.apiErrorDetail(r, w, http.StatusServiceUnavailable, apiErrorDetail{
			Code: codeUnavailable,
			Message: "The system is busy or briefly unavailable. " +
				"Please try again in a moment.",
			Violations: nil, Malformed: nil,
		}, nil, extra...)

		return
	}

	status, detail, ok := dbErrorDetail(err, deleting)
	if !ok {
		app.apiInternalError(r, w, err, extra...)

		return
	}

	app.apiErrorDetail(r, w, status, detail, err, extra...)
}

// transientFailure reports whether an error means the database could
// not answer, as opposed to answering that the request was wrong.
//
// Both shapes are matched structurally rather than by message text.
// A database that cannot be reached at all surfaces as
// pgconn.ConnectError — the pool has no connection and cannot make
// one. A database that is reachable but did not answer within the
// ceiling surfaces as a deadline: either the read ceiling, the write
// timeout, or the pool refusing to hand out a connection because every
// one of them is busy. All three mean "come back shortly".
//
// A connection that dies in the middle of a query is not matched: pgx
// surfaces it as a bare error whose only distinguishing feature is its
// message, and guessing from prose is worse than the honest 500 it
// gets instead. It costs one request, because the pool discards that
// connection and the next attempt fails as a ConnectError.
func transientFailure(err error) (bool, string) {
	if _, unreachable := errors.AsType[*pgconn.ConnectError](err); unreachable {
		return true, "unreachable"
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true, "timed out"
	}

	return false, ""
}
