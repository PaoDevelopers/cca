package web //nolint:testpackage

import (
	"archive/zip"
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// upload builds the multipart request an administrator's browser
// sends, so these go through the real reader rather than a stub of it.
func upload(t *testing.T, body string) *http.Request {
	t.Helper()

	return uploadFile(t, "enrollments.csv", []byte(body))
}

func uploadFile(t *testing.T, filename string, body []byte) *http.Request {
	t.Helper()

	var buf bytes.Buffer

	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("spreadsheet", filename)
	if err != nil {
		t.Fatalf("build the upload: %v", err)
	}

	if _, err := part.Write(body); err != nil {
		t.Fatalf("write the upload: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close the upload: %v", err)
	}

	r := httptest.NewRequestWithContext(t.Context(),
		http.MethodPost, "/admin/api/enrollments/import", &buf)
	r.Header.Set("Content-Type", writer.FormDataContentType())

	return r
}

type testXLSXSheet struct {
	name     string
	rows     [][]string
	hidden   bool
	formulas map[string]string
}

func xlsxBytes(t *testing.T, sheets ...testXLSXSheet) []byte {
	t.Helper()

	if len(sheets) == 0 {
		t.Fatal("an XLSX test workbook needs a sheet")
	}

	book := excelize.NewFile()
	defer func() {
		_ = book.Close()
	}()

	for i, sheet := range sheets {
		if i == 0 {
			if err := book.SetSheetName("Sheet1", sheet.name); err != nil {
				t.Fatalf("name XLSX sheet: %v", err)
			}
		} else if _, err := book.NewSheet(sheet.name); err != nil {
			t.Fatalf("add XLSX sheet: %v", err)
		}

		for rowIndex, row := range sheet.rows {
			cell := "A" + strconv.Itoa(rowIndex+1)
			if err := book.SetSheetRow(sheet.name, cell, &row); err != nil {
				t.Fatalf("write XLSX row %s!%s: %v", sheet.name, cell, err)
			}
		}

		for cell, formula := range sheet.formulas {
			if err := book.SetCellFormula(sheet.name, cell, formula); err != nil {
				t.Fatalf("write XLSX formula %s!%s: %v", sheet.name, cell, err)
			}
		}

		if sheet.hidden {
			if err := book.SetSheetVisible(sheet.name, false); err != nil {
				t.Fatalf("hide XLSX sheet %s: %v", sheet.name, err)
			}
		}
	}

	buf, err := book.WriteToBuffer()
	if err != nil {
		t.Fatalf("encode XLSX workbook: %v", err)
	}

	return buf.Bytes()
}

// The export is the PowerSchool hand-off and the import is four
// columns, so the two do not match — which is fine until an
// administrator does the obvious thing and uploads the file they were
// handed. Both shapes are read; the wide one is reduced.
func TestTheEnrollmentExportCanBeImportedBack(t *testing.T) {
	t.Parallel()

	// What the export produces for two students. Alice's Chess meets
	// in two periods, so it is two rows for one enrollment; Bob's
	// course has none, so its period cell is empty.
	exported := strings.Join([]string{
		strings.Join(enrollmentExportColumns, ","),
		"s1001,Alice,Y9,F,CH,Chess,Year,MON1,true,true",
		"s1001,Alice,Y9,F,CH,Chess,Year,TUE2,true,true",
		"s1002,Bob,Y9,M,BK,Baking,Season,,false,true",
	}, "\n") + "\n"

	rows, shape, err := readSpreadsheetUploadAny(httptest.NewRecorder(),
		upload(t, exported), enrollmentImportColumns, enrollmentExportColumns)
	if err != nil {
		t.Fatalf("the file the export produced was refused: %v", err)
	}

	if shape != 1 {
		t.Fatalf("shape = %d, want the export's", shape)
	}

	reduced := reduceExportRows(rows)

	want := [][]string{
		{"CH", "s1001", "true", "true"},
		{"BK", "s1002", "false", "true"},
	}
	if !slices.EqualFunc(reduced, want, slices.Equal) {
		t.Errorf("reduced to %v, want %v", reduced, want)
	}

	// Placing an enrollment twice is a duplicate key, so the collapse
	// is the whole of whether the round trip works.
	groups, err := groupEnrollmentRows(reduced)
	if err != nil {
		t.Fatalf("group: %v", err)
	}

	for _, group := range groups {
		if len(group.studentIDs) != len(slices.Compact(slices.Clone(group.studentIDs))) {
			t.Errorf("course %s names a student twice: %v", group.courseID, group.studentIDs)
		}
	}
}

// The narrow shape is still the canonical one and still works.
func TestTheEnrollmentImportShapeStillReads(t *testing.T) {
	t.Parallel()

	body := strings.Join([]string{
		strings.Join(enrollmentImportColumns, ","),
		"CH,s1001,true,true",
	}, "\n") + "\n"

	rows, shape, err := readSpreadsheetUploadAny(httptest.NewRecorder(),
		upload(t, body), enrollmentImportColumns, enrollmentExportColumns)
	if err != nil {
		t.Fatalf("the import shape was refused: %v", err)
	}

	if shape != 0 {
		t.Errorf("shape = %d, want the import's", shape)
	}

	if len(rows) != 1 || rows[0][0] != "CH" {
		t.Errorf("rows = %v", rows)
	}
}

// A file that is neither is still refused, and the message names the
// canonical shape rather than leaving the administrator to guess which
// of two it failed to be.
func TestAnUnrecognisedHeaderIsRefusedAgainstTheCanonicalShape(t *testing.T) {
	t.Parallel()

	body := "course,student,droppable,budgeted\nCH,s1001,true,true\n"

	_, _, err := readSpreadsheetUploadAny(httptest.NewRecorder(),
		upload(t, body), enrollmentImportColumns, enrollmentExportColumns)
	if err == nil {
		t.Fatal("a file with the wrong columns was accepted")
	}

	if !strings.Contains(err.Error(), "course_id") {
		t.Errorf("the message does not name the expected column: %v", err)
	}
}

// The guard against a formula is applied to every cell on the way out
// and undone on the way in, so a name that starts with one of the
// dangerous characters survives the trip unchanged.
func TestAFormulaLikeNameSurvivesTheRoundTrip(t *testing.T) {
	t.Parallel()

	// A real one: a course named for a range, and a teacher whose name
	// a spreadsheet would read as a subtraction.
	for _, cell := range []string{"=Rowing", "-Smith", "@Home", "+Plus"} {
		escaped := csvEscapeCell(cell)
		if strings.ContainsRune("=+-@", rune(escaped[0])) {
			t.Errorf("%q would still be evaluated", escaped)
		}

		if back := csvUnescapeCell(escaped); back != cell {
			t.Errorf("%q came back as %q", cell, back)
		}
	}
}

// The file an administrator at a Chinese school will eventually upload.
//
// Excel on a Chinese-language Windows machine writes GB18030 when you
// choose "CSV", and encoding/csv does not validate anything — it
// copies bytes into strings — so this parsed cleanly and failed much
// further along, where nothing could name the cause. Through the
// roster it became PostgreSQL's 22021 and the words "That value is not
// valid here", which send someone hunting for a bad cell that does not
// exist. Through the catalogue it became a 500, because json/v2
// refuses to encode invalid UTF-8 and that error is not a
// *pgconn.PgError.
func TestAFileInTheWrongEncodingSaysSo(t *testing.T) {
	t.Parallel()

	// "李娜" as GB18030: c0 ee c4 c8, which is not valid UTF-8.
	body := strings.Join(enrollmentImportColumns, ",") +
		"\nCH,\xc0\xee\xc4\xc8,true,true\n"

	_, _, err := readSpreadsheetUploadAny(httptest.NewRecorder(), upload(t, body),
		enrollmentImportColumns)
	if err == nil {
		t.Fatal("a GB18030 file was accepted")
	}

	if !errors.Is(err, errBadCell) {
		t.Errorf("error is %v, which will not become a 400", err)
	}

	for _, want := range []string{"row 2", "column 2", "UTF-8", "GB18030"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not mention %q: %v", want, err)
		}
	}
}

// A NUL byte is valid UTF-8 and PostgreSQL cannot store it. It used to
// reach the jsonb cast and come back as 22P05, "unsupported Unicode
// escape sequence", about a \u0000 nobody typed — and 22P05 was in no
// case arm, so it was another 500.
func TestANulByteIsRefusedWhereItCanStillBeExplained(t *testing.T) {
	t.Parallel()

	_, _, err := readSpreadsheetUploadAny(httptest.NewRecorder(),
		upload(t, strings.Join(enrollmentImportColumns, ",")+
			"\nCH,s10\x0001,true,true\n"),
		enrollmentImportColumns)
	if err == nil {
		t.Fatal("a cell holding NUL was accepted")
	}

	if !errors.Is(err, errBadCell) {
		t.Errorf("error is %v, which will not become a 400", err)
	}

	if !strings.Contains(err.Error(), "NUL") {
		t.Errorf("message does not name the byte: %v", err)
	}
}

// The encoding check runs before the header is matched, so a file in
// the wrong encoding is not reported as having unrecognisable columns.
func TestABadEncodingInTheHeaderIsReportedAsAnEncoding(t *testing.T) {
	t.Parallel()

	_, _, err := readSpreadsheetUploadAny(httptest.NewRecorder(),
		upload(t, "course_id,\xc0\xee,student_droppable,"+
			"counts_toward_budget\nCH,s1001,true,true\n"),
		enrollmentImportColumns)
	if err == nil {
		t.Fatal("a header in the wrong encoding was accepted")
	}

	if !strings.Contains(err.Error(), "UTF-8") {
		t.Errorf("the header was judged on its column names, not its "+
			"bytes: %v", err)
	}
}

func TestXLSXImportReadsTheCSVShapeFromOneVisibleSheet(t *testing.T) {
	t.Parallel()

	body := xlsxBytes(t,
		testXLSXSheet{
			name: "Notes",
			rows: [][]string{{"Paste the roster on the other sheet."}},
		},
		testXLSXSheet{
			name: "Roster",
			rows: [][]string{
				studentImportColumns,
				{" s22537 ", "=Alice", "Y12", "X"},
			},
		},
	)

	rows, shape, err := readSpreadsheetUploadAny(httptest.NewRecorder(),
		uploadFile(t, "students.xlsx", body), studentImportColumns)
	if err != nil {
		t.Fatalf("read XLSX: %v", err)
	}

	if shape != 0 {
		t.Errorf("shape = %d, want 0", shape)
	}

	want := [][]string{{"s22537", "=Alice", "Y12", "X"}}
	if !slices.EqualFunc(rows, want, slices.Equal) {
		t.Errorf("rows = %v, want %v", rows, want)
	}
}

func TestXLSXImportPadsTrailingEmptyCells(t *testing.T) {
	t.Parallel()

	body := xlsxBytes(t, testXLSXSheet{
		name: "Courses",
		rows: [][]string{courseImportColumns, {"CH", "Chess"}},
	})

	rows, _, err := readSpreadsheetUploadAny(httptest.NewRecorder(),
		uploadFile(t, "courses.xlsx", body), courseImportColumns)
	if err != nil {
		t.Fatalf("read XLSX: %v", err)
	}

	if len(rows) != 1 || len(rows[0]) != len(courseImportColumns) {
		t.Fatalf("rows = %v, want one row with %d cells", rows, len(courseImportColumns))
	}

	if rows[0][0] != "CH" || rows[0][1] != "Chess" || !allCellsEmpty(rows[0][2:]) {
		t.Errorf("padded row = %v", rows[0])
	}
}

func TestXLSXImportRejectsAmbiguousDataSheets(t *testing.T) {
	t.Parallel()

	body := xlsxBytes(t,
		testXLSXSheet{name: "Roster A", rows: [][]string{studentImportColumns}},
		testXLSXSheet{name: "Roster B", rows: [][]string{studentImportColumns}},
	)

	_, _, err := readSpreadsheetUploadAny(httptest.NewRecorder(),
		uploadFile(t, "students.xlsx", body), studentImportColumns)
	if err == nil {
		t.Fatal("two matching worksheets were accepted")
	}

	for _, name := range []string{"Roster A", "Roster B"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not name %q: %v", name, err)
		}
	}
}

func TestXLSXImportIgnoresHiddenMatchingSheets(t *testing.T) {
	t.Parallel()

	body := xlsxBytes(t,
		testXLSXSheet{
			name: "Current roster",
			rows: [][]string{studentImportColumns, {"s22537", "Alice", "Y12", "X"}},
		},
		testXLSXSheet{
			name:   "Old roster",
			hidden: true,
			rows:   [][]string{studentImportColumns, {"s14003", "Bob", "Y9", "M"}},
		},
	)

	rows, _, err := readSpreadsheetUploadAny(httptest.NewRecorder(),
		uploadFile(t, "students.xlsx", body), studentImportColumns)
	if err != nil {
		t.Fatalf("read XLSX: %v", err)
	}

	if len(rows) != 1 || rows[0][0] != "s22537" {
		t.Errorf("rows = %v, want only the visible roster", rows)
	}
}

func TestXLSXImportRejectsFormulas(t *testing.T) {
	t.Parallel()

	body := xlsxBytes(t, testXLSXSheet{
		name: "Roster",
		rows: [][]string{
			studentImportColumns,
			{"s22537", "Alice", "Y12", "X"},
		},
		formulas: map[string]string{"B2": `"Alice"`},
	})

	_, _, err := readSpreadsheetUploadAny(httptest.NewRecorder(),
		uploadFile(t, "students.xlsx", body), studentImportColumns)
	if err == nil {
		t.Fatal("an XLSX formula was accepted")
	}

	for _, want := range []string{"row 2", "column 2", "formula", "paste values"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("formula error does not mention %q: %v", want, err)
		}
	}
}

func TestMacroEnabledWorkbookExtensionIsRefused(t *testing.T) {
	t.Parallel()

	body := xlsxBytes(t, testXLSXSheet{
		name: "Roster",
		rows: [][]string{studentImportColumns},
	})

	_, _, err := parseSpreadsheet("students.xlsm", body, studentImportColumns)
	if !errors.Is(err, errUnsupportedSpreadsheet) {
		t.Errorf("XLSM error = %v, want errUnsupportedSpreadsheet", err)
	}
}

func TestXLSXArchiveExpansionIsBounded(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer

	writer := zip.NewWriter(&body)

	part, err := writer.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		t.Fatalf("create ZIP part: %v", err)
	}

	if _, err := part.Write(bytes.Repeat([]byte{'x'}, 65)); err != nil {
		t.Fatalf("write ZIP part: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close ZIP: %v", err)
	}

	err = validateXLSXArchive(body.Bytes(), 64, 10)
	if !errors.Is(err, errUploadTooLarge) {
		t.Errorf("expanded XLSX error = %v, want errUploadTooLarge", err)
	}
}
