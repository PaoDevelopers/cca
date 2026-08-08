package web

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

// The shared shape of the three spreadsheet imports. Each of them used
// to carry its own copy of the multipart handling, the byte-order-mark
// dance and the header check, which is three chances for them to
// disagree about what a CSV is.

// spreadsheetUploadLimit bounds an uploaded spreadsheet. A full-school
// roster of 1200 students is well under a megabyte; this is generous
// enough that a legitimate file never hits it and small enough that a
// hostile one cannot exhaust memory.
const spreadsheetUploadLimit = 8 << 20

// spreadsheetUploadTimeout bounds how long the upload may take to
// arrive. A full roster over a slow school connection is comfortably
// inside it.
const spreadsheetUploadTimeout = 2 * time.Minute

const (
	// An XLSX is a ZIP, and a small upload can expand to a workbook much
	// larger than the file that crossed the network. These limits are
	// deliberately far above a school-sized import but below the point
	// where XML expansion can crowd out the server.
	xlsxUnzipSizeLimit    = 64 << 20
	xlsxUnzipXMLSizeLimit = 8 << 20
	xlsxPartLimit         = 4096

	// Compression makes a million nearly-empty rows cheap to upload.
	// The CSV body limit used to be the effective row limit; XLSX needs
	// an explicit equivalent. This includes the header row.
	xlsxRowLimit = 100001
)

var (
	errEmptySpreadsheet       = errors.New("the file has no header row")
	errUnsupportedSpreadsheet = errors.New("unsupported spreadsheet format")

	// The columns are the file's contract with the administrator, so
	// a mismatch names both what was found and what was wanted.
	errWrongColumn = errors.New("unexpected column")

	// A cell whose text cannot be read as the type its column
	// promises. Distinct from a rule rejection: nothing about it is
	// negotiable, and the database never sees it.
	errBadCell = errors.New("malformed cell")

	// The upload ran over the limit. Its own error because the answer
	// is a different status and a different thing to tell the user:
	// "too big" is actionable where "malformed" is not.
	errUploadTooLarge = errors.New("the file is too large")
)

// readSpreadsheetUpload pulls the "spreadsheet" file part out of a
// multipart request, checks its header against the expected columns,
// and returns the remaining rows.
//
// The whole file is read before anything is written to the database.
// A spreadsheet is small, and holding it costs less than discovering
// on row 900 that the file was truncated after the writes for rows 1
// to 899 have already gone in.
func readSpreadsheetUpload(w http.ResponseWriter, r *http.Request, expected []string) ([][]string, error) {
	rows, _, err := readSpreadsheetUploadAny(w, r, expected)

	return rows, err
}

// readSpreadsheetUploadAny is readSpreadsheetUpload for a file that
// may legitimately arrive in more than one shape, and reports which
// of them it was.
//
// The enrollments spreadsheet is the reason. What the export produces
// is not what the import wants — the export carries names, terms and
// one row per period, because it is the hand-off to PowerSchool, while
// the import wants the four columns that actually decide anything. An
// administrator does not know that, and has no reason to: they export,
// edit in a spreadsheet, and upload the file they were given back. So
// both shapes are accepted, and the caller maps whichever arrived.
func readSpreadsheetUploadAny(
	w http.ResponseWriter, r *http.Request, shapes ...[]string,
) ([][]string, int, error) {
	// A spreadsheet is bigger than a JSON body, so it gets longer —
	// but it still gets a deadline. See boundBodyRead.
	boundBodyRead(w, spreadsheetUploadTimeout)

	r.Body = http.MaxBytesReader(w, r.Body, spreadsheetUploadLimit)

	if err := r.ParseMultipartForm(spreadsheetUploadLimit); err != nil { //#nosec:G120
		if _, tooBig := errors.AsType[*http.MaxBytesError](err); tooBig {
			return nil, 0, fmt.Errorf("%w: the limit is %d bytes",
				errUploadTooLarge, spreadsheetUploadLimit)
		}

		return nil, 0, fmt.Errorf("read upload: %w", err)
	}

	// Parts larger than the in-memory budget are spilled to files in
	// the temp directory, and closing the part does not remove them.
	//
	// Nothing can reach that today: the memory budget passed to
	// ParseMultipartForm and the cap on the whole body are the same
	// constant, so a part that could spill would have been rejected as
	// too large first. Kept because it is one line and the alternative
	// is a temp file per upload the day those two numbers differ.
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	f, header, err := r.FormFile("spreadsheet")
	if err != nil {
		return nil, 0, fmt.Errorf("a CSV or XLSX file is required: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, 0, fmt.Errorf("read uploaded spreadsheet: %w", err)
	}

	return parseSpreadsheet(header.Filename, data, shapes...)
}

func parseSpreadsheet(filename string, data []byte, shapes ...[]string) ([][]string, int, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".xls", ".xlsm":
		return nil, 0, fmt.Errorf("%w %q; use CSV or XLSX", errUnsupportedSpreadsheet, ext)
	case ".xlsx":
		return parseXLSX(data, shapes...)
	}

	if len(data) >= 4 && bytes.Equal(data[:4], []byte{'P', 'K', 3, 4}) {
		return parseXLSX(data, shapes...)
	}

	return parseCSV(data, shapes...)
}

func parseCSV(data []byte, shapes ...[]string) ([][]string, int, error) {
	br := bufio.NewReader(bytes.NewReader(data))

	// Excel writes a byte-order mark and then calls the result UTF-8.
	// Left in place it becomes part of the first header cell, and the
	// header check fails with a message about a column that looks
	// identical to the one expected.
	if b, _ := br.Peek(3); len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		if _, err := br.Discard(3); err != nil {
			return nil, 0, fmt.Errorf("skip byte-order mark: %w", err)
		}
	}

	reader := csv.NewReader(br)
	// Every row must have the same shape as the header; the encoding
	// package enforces that for us once FieldsPerRecord is set from
	// the header read. Which shape that is comes from the header, so
	// it is set after reading it rather than before.
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, 0, errEmptySpreadsheet
		}

		return nil, 0, fmt.Errorf("read header: %w", err)
	}

	// Before the header is matched, so that a file in the wrong
	// encoding is reported as being in the wrong encoding rather than
	// as having unrecognisable column names.
	if err := checkSpreadsheetText(-1, header); err != nil {
		return nil, 0, err
	}

	shape := -1

	for i, expected := range shapes {
		if headerMatches(header, expected) {
			shape = i

			break
		}
	}

	if shape < 0 {
		// The first shape is the one to describe: it is the canonical
		// one, and where several are accepted the others are
		// tolerated spellings of it rather than equal alternatives.
		return nil, 0, headerMismatch(header, shapes[0])
	}

	reader.FieldsPerRecord = len(shapes[shape])

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, 0, fmt.Errorf("read rows: %w", err)
	}

	// Trimming here rather than at every use: a spreadsheet's cells
	// pick up stray spaces constantly, and the identifier domains
	// reject them, so an untrimmed import fails for a reason the
	// administrator cannot see in their editor.
	for index, row := range rows {
		if err := checkSpreadsheetText(index, row); err != nil {
			return nil, 0, err
		}

		for i := range row {
			row[i] = csvUnescapeCell(strings.TrimSpace(row[i]))
		}
	}

	return rows, shape, nil
}

type xlsxCandidate struct {
	name  string
	shape int
}

func parseXLSX(data []byte, shapes ...[]string) ([][]string, int, error) {
	if err := validateXLSXArchive(data, xlsxUnzipSizeLimit, xlsxPartLimit); err != nil {
		return nil, 0, err
	}

	tmpDir, err := os.MkdirTemp("", "cca-xlsx-")
	if err != nil {
		return nil, 0, fmt.Errorf("create XLSX workspace: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	//exhaustruct:ignore
	book, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		UnzipSizeLimit:    xlsxUnzipSizeLimit,
		UnzipXMLSizeLimit: xlsxUnzipXMLSizeLimit,
		TmpDir:            tmpDir,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("read XLSX workbook: %w", err)
	}
	defer func() {
		_ = book.Close()
	}()

	candidates, firstHeader, err := matchingXLSXSheets(book, shapes...)
	if err != nil {
		return nil, 0, err
	}

	switch len(candidates) {
	case 0:
		if firstHeader == nil {
			return nil, 0, errEmptySpreadsheet
		}

		return nil, 0, headerMismatch(firstHeader, shapes[0])
	case 1:
		// The one unambiguous data sheet is the XLSX equivalent of the
		// one table a CSV can contain.
	default:
		names := make([]string, len(candidates))
		for i, candidate := range candidates {
			names[i] = candidate.name
		}

		return nil, 0, fmt.Errorf("%w: more than one visible worksheet has the expected header: %s",
			errWrongColumn, strings.Join(names, ", "))
	}

	candidate := candidates[0]

	rows, err := readXLSXSheet(book, candidate.name, shapes[candidate.shape])
	if err != nil {
		return nil, 0, err
	}

	return rows, candidate.shape, nil
}

func validateXLSXArchive(data []byte, unzipLimit uint64, partLimit int) error {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("read XLSX archive: %w", err)
	}

	if len(archive.File) > partLimit {
		return fmt.Errorf("%w: XLSX contains %d parts, the limit is %d",
			errUploadTooLarge, len(archive.File), partLimit)
	}

	var expanded uint64
	for _, part := range archive.File {
		if part.UncompressedSize64 > unzipLimit-expanded {
			return fmt.Errorf("%w: XLSX expands beyond the %d byte limit",
				errUploadTooLarge, unzipLimit)
		}

		expanded += part.UncompressedSize64
	}

	return nil
}

func matchingXLSXSheets(book *excelize.File, shapes ...[]string) ([]xlsxCandidate, []string, error) {
	var (
		candidates  []xlsxCandidate
		firstHeader []string
	)

	for _, name := range book.GetSheetList() {
		visible, err := book.GetSheetVisible(name)
		if err != nil {
			return nil, nil, fmt.Errorf("read worksheet %q visibility: %w", name, err)
		}

		if !visible {
			continue
		}

		rows, err := book.Rows(name)
		if err != nil {
			if _, notWorksheet := errors.AsType[excelize.ErrSheetNotExist](err); notWorksheet {
				continue
			}

			return nil, nil, fmt.Errorf("read worksheet %q: %w", name, err)
		}

		var header []string
		if rows.Next() {
			header, err = rows.Columns()
		}

		closeErr := rows.Close()

		if err != nil {
			return nil, nil, fmt.Errorf("read worksheet %q header: %w", name, err)
		}

		if closeErr != nil {
			return nil, nil, fmt.Errorf("close worksheet %q: %w", name, closeErr)
		}

		if header == nil {
			continue
		}

		if err := checkSpreadsheetText(-1, header); err != nil {
			return nil, nil, err
		}

		if firstHeader == nil {
			firstHeader = slices.Clone(header)
		}

		for shape, expected := range shapes {
			if headerMatches(header, expected) {
				candidates = append(candidates, xlsxCandidate{name: name, shape: shape})

				break
			}
		}
	}

	return candidates, firstHeader, nil
}

type xlsxUsedRow struct {
	number  int
	columns int
}

func readXLSXSheet(book *excelize.File, sheet string, expected []string) ([][]string, error) {
	iterator, err := book.Rows(sheet)
	if err != nil {
		return nil, fmt.Errorf("read worksheet %q: %w", sheet, err)
	}

	var (
		rows     [][]string
		usedRows []xlsxUsedRow
	)

	rowNumber := 0

	for iterator.Next() {
		rowNumber++
		if rowNumber > xlsxRowLimit {
			_ = iterator.Close()

			return nil, fmt.Errorf("%w: XLSX worksheet has more than %d data rows",
				errUploadTooLarge, xlsxRowLimit-1)
		}

		row, readErr := iterator.Columns()
		if readErr != nil {
			_ = iterator.Close()

			return nil, fmt.Errorf("read worksheet %q row %d: %w", sheet, rowNumber, readErr)
		}

		if len(row) > len(expected) {
			_ = iterator.Close()

			return nil, fmt.Errorf("%w: row %d has a value in column %d; expected %d columns",
				errBadCell, rowNumber, len(row), len(expected))
		}

		if len(row) > 0 {
			usedRows = append(usedRows, xlsxUsedRow{number: rowNumber, columns: len(row)})
		}

		if rowNumber == 1 {
			continue
		}

		normalized := make([]string, len(expected))
		copy(normalized, row)

		if err := checkSpreadsheetText(rowNumber-2, normalized); err != nil {
			_ = iterator.Close()

			return nil, err
		}

		for i := range normalized {
			normalized[i] = strings.TrimSpace(normalized[i])
		}

		if !allCellsEmpty(normalized) {
			rows = append(rows, normalized)
		}
	}

	if err := iterator.Close(); err != nil {
		return nil, fmt.Errorf("close worksheet %q: %w", sheet, err)
	}

	if err := rejectXLSXFormulas(book, sheet, usedRows); err != nil {
		return nil, err
	}

	return rows, nil
}

func rejectXLSXFormulas(book *excelize.File, sheet string, rows []xlsxUsedRow) error {
	for _, row := range rows {
		for column := 1; column <= row.columns; column++ {
			cell, err := excelize.CoordinatesToCellName(column, row.number)
			if err != nil {
				return fmt.Errorf("name XLSX cell at row %d column %d: %w", row.number, column, err)
			}

			formula, err := book.GetCellFormula(sheet, cell)
			if err != nil {
				return fmt.Errorf("read XLSX formula in %s!%s: %w", sheet, cell, err)
			}

			if formula != "" {
				return fmt.Errorf("%w: row %d column %d contains a formula; paste values before importing",
					errBadCell, row.number, column)
			}
		}
	}

	return nil
}

func allCellsEmpty(row []string) bool {
	return !slices.ContainsFunc(row, func(cell string) bool {
		return cell != ""
	})
}

func headerMatches(header []string, expected []string) bool {
	if len(header) != len(expected) {
		return false
	}

	for i, col := range header {
		if strings.TrimSpace(col) != expected[i] {
			return false
		}
	}

	return true
}

// headerMismatch names the first thing that is wrong, because a
// spreadsheet with the wrong columns usually has exactly one thing
// wrong with it and listing all ten buries it.
func headerMismatch(header []string, expected []string) error {
	for i, want := range expected {
		if i >= len(header) {
			return fmt.Errorf("%w: column %d is missing, expected %q",
				errWrongColumn, i+1, want)
		}

		if got := strings.TrimSpace(header[i]); got != want {
			return fmt.Errorf("%w: column %d is %q, expected %q",
				errWrongColumn, i+1, got, want)
		}
	}

	return fmt.Errorf("%w: %d columns, expected %d",
		errWrongColumn, len(header), len(expected))
}

// A spreadsheet is a program, and a cell is one of its expressions.
//
// Excel, LibreOffice and Sheets all read a cell beginning with "=",
// "+", "-" or "@" as a formula, and a formula can reach outside the
// document: =HYPERLINK() to exfiltrate the row it sits next to,
// =WEBSERVICE() to fetch, and DDE to launch. Our exports carry names
// that came from an upload, so a course called "=cmd|..." is a payload
// this program hands to an administrator's machine, signed by being a
// file they asked for.
//
// The usual mitigation — prefix a quote — breaks the round trip, which
// is the point of these exports. So it is an encoding rather than a
// mangling: any cell that would be read as a formula, and any cell
// that already starts with the escape, gets one apostrophe in front,
// and the import strips exactly one. Both directions are total, so the
// escaping itself is invisible across an export-edit-reimport.
//
// The round trip as a whole is idempotent rather than the identity:
// the import also trims each cell (see below), so a value that carried
// leading or trailing whitespace comes back without it, and unchanged
// from then on. For every column but one that is no loss, because the
// text domains refuse such a value anyway. The exception is
// `description`, which is plain TEXT and may be several paragraphs: a
// blank line at its start or end does not survive. Blank lines *in* it
// do, and so does every indent that is not at an edge.
//
// (The apostrophe is also what a spreadsheet itself writes to mean
// "this is text", so the exported file looks right when opened.)
const csvFormulaLead = "=+-@\t\r'"

func csvEscapeCell(cell string) string {
	if cell == "" || !strings.ContainsRune(csvFormulaLead, rune(cell[0])) {
		return cell
	}

	return "'" + cell
}

func csvUnescapeCell(cell string) string {
	return strings.TrimPrefix(cell, "'")
}

// checkSpreadsheetText refuses a row whose bytes cannot become text.
//
// encoding/csv does not validate anything: it copies bytes into
// strings. So a GB18030 file — which is what Excel writes when you
// save as CSV on a Chinese Windows machine, and therefore what arrives
// here — parsed without complaint and failed much later, somewhere
// that could not name the cause. The students import reached
// PostgreSQL and came back as 22021, which the administrator was shown
// as "That value is not valid here", sending them to hunt for a bad
// cell that does not exist. The courses import never reached
// PostgreSQL at all: json/v2 refuses to encode invalid UTF-8, and that
// error is not a *pgconn.PgError, so it fell through to a 500 and an
// error-level log for an ordinary wrong-encoding file.
//
// Checking here makes it one message, at the top of the import, naming
// the file and the likely reason. Note what it cannot catch: 1820
// GB18030 characters — including the surnames 鲁, 陆, 卢, 娄 and 路 —
// have byte sequences that are also valid UTF-8, so those cells are
// accepted and stored as mojibake. Detecting that needs the
// administrator to say what encoding they meant; refusing what is
// certainly wrong is what can be done without asking.
//
// NUL goes in the same pass. It is valid UTF-8 and PostgreSQL cannot
// store it, so it surfaced as 22P05 — "unsupported Unicode escape
// sequence" about a \u0000 nobody typed — which no case arm handled,
// making it another 500.
func checkSpreadsheetText(index int, row []string) error {
	for i, cell := range row {
		if !utf8.ValidString(cell) {
			return rowError(index, fmt.Sprintf(
				"column %d is not UTF-8 text. A spreadsheet saved as "+
					"CSV on a Chinese-language Windows machine is "+
					"usually GB18030; re-save it as \"CSV UTF-8\".",
				i+1))
		}

		if strings.ContainsRune(cell, 0) {
			return rowError(index, fmt.Sprintf(
				"column %d contains a NUL byte, which no text field "+
					"can hold", i+1))
		}
	}

	return nil
}

// rowError names the spreadsheet line a parse failure came from, in
// the numbering the administrator sees in their editor.
func rowError(index int, message string) error {
	return fmt.Errorf("%w: row %d: %s", errBadCell, csvRowNumber(index), message)
}

// csvRowNumber converts a zero-based index into the data rows to the
// line number the administrator sees in their spreadsheet, where row 1
// is the header. Index -1 is the header itself.
func csvRowNumber(index int) int {
	return index + 2
}

// parseBoolCell reads the spellings a spreadsheet actually contains.
// Excel writes TRUE and FALSE; people write yes and no; an empty cell
// means false, because an unfilled column should not assert anything.
//
// Used where the value has to be a Go bool before it reaches the
// database. Where the database casts the cell itself — the course
// import — the text is passed through untouched instead, so that a
// bad boolean is collected with every other bad cell rather than
// rejecting the file on its own.
func parseBoolCell(cell string) (bool, error) {
	switch cell {
	case "", "0", "false", "FALSE", "False", "no", "NO", "No", "n", "N":
		return false, nil
	case "1", "true", "TRUE", "True", "yes", "YES", "Yes", "y", "Y":
		return true, nil
	}

	return false, fmt.Errorf("%w: expected true or false, got %s",
		errBadCell, strconv.Quote(cell))
}

// splitList parses one cell holding several ids. Empty means the empty
// list, not a list holding one empty id.
func splitList(cell string) []string {
	if cell == "" {
		return nil
	}

	parts := strings.Split(cell, ",")

	out := make([]string, 0, len(parts))

	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}

	return out
}

// spreadsheetUploadError answers a rejected upload. A file over the
// limit gets its own status, because "too large" is something the
// administrator can act on and "malformed" is not.
func (app *Server) spreadsheetUploadError(r *http.Request, w http.ResponseWriter, err error, extra ...slog.Attr) {
	if errors.Is(err, errUploadTooLarge) {
		app.apiError(r, w, http.StatusRequestEntityTooLarge,
			codeBadRequest, err.Error(), err, extra...)

		return
	}

	app.apiBadRequest(r, w, err.Error(), err, extra...)
}
