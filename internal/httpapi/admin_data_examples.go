package httpapi

import (
	"log/slog"
	"net/http"
	"os"
)

type dataExample struct {
	path      string
	baseName  string
	sheetName string
}

var dataExamples = map[string]dataExample{
	"courses": {
		path:      "database/fixtures/import-examples/courses.csv",
		baseName:  "courses_example",
		sheetName: "Courses",
	},
	"students": {
		path:      "database/fixtures/import-examples/students.csv",
		baseName:  "students_example",
		sheetName: "Students",
	},
	"selections": {
		path:      "database/fixtures/import-examples/selections.csv",
		baseName:  "selections_example",
		sheetName: "Selections",
	},
}

func (app *App) handleAdmDataExample(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmDataExample", slog.String("admin_username", aui.Username))
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil, slog.String("admin_username", aui.Username))
		return
	}

	example, ok := dataExamples[r.PathValue("kind")]
	if !ok {
		app.apiError(r, w, http.StatusNotFound, nil, slog.String("admin_username", aui.Username))
		return
	}
	format, err := parseTabularFormat(r.URL.Query().Get("format"))
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	file, err := os.Open(example.path)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nfailed opening example data", err, slog.String("admin_username", aui.Username))
		return
	}
	defer func() {
		_ = file.Close()
	}()
	rows, err := readAllTabularRows(tabularFormatCSV, file)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nfailed reading example data", err, slog.String("admin_username", aui.Username))
		return
	}
	if err := writeTabularDownload(w, format, example.baseName, example.sheetName, rows); err != nil {
		app.logWarn(r, logMsgHTTPResponseError, slog.Any("error", err), slog.String("admin_username", aui.Username))
	}
}
