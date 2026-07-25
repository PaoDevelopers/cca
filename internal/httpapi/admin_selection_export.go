package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"
)

func (app *App) handleAdmSelectionsExport(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmSelectionsExport", slog.String("admin_username", aui.Username))
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil, slog.String("admin_username", aui.Username))
		return
	}
	format, err := parseTabularFormat(r.URL.Query().Get("format"))
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	rows, err := app.queries.GetSelectionsExport(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	records := make([][]string, 0, len(rows)+1)
	records = append(records, []string{"student_id", "student_name", "grade", "legal_sex", "course_id", "course_name", "periods", "selection_type"})
	for _, row := range rows {
		records = append(records, []string{
			strconv.FormatInt(row.StudentID, 10),
			row.StudentName,
			row.Grade,
			string(row.LegalSex),
			row.CourseID,
			row.CourseName,
			row.Periods,
			string(row.SelectionType),
		})
	}
	if err := writeTabularDownload(w, format, "selections", "Selections", records); err != nil {
		app.logWarn(r, logMsgHTTPResponseError, slog.Any("error", err), slog.String("admin_username", aui.Username))
	}
	app.logInfo(r, logMsgAdminSelectionsExport, slog.String("admin_username", aui.Username), slog.Int("row_count", len(rows)), slog.String("format", string(format)))
}
