package transfer

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Format identifies a supported import or export file type.
type Format string

const (
	// FormatCSV selects comma-separated values.
	FormatCSV Format = "csv"
	// FormatXLSX selects an Excel workbook.
	FormatXLSX Format = "xlsx"

	maxWorkbookUncompressedSize int64 = 64 << 20
	maxWorkbookXMLMemory        int64 = 8 << 20
)

// ParseFormat parses a request value, defaulting to CSV.
func ParseFormat(value string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(FormatCSV):
		return FormatCSV, nil
	case string(FormatXLSX), "excel":
		return FormatXLSX, nil
	default:
		return "", fmt.Errorf("unsupported data format %q", value)
	}
}

func (f Format) extension() string {
	if f == FormatXLSX {
		return ".xlsx"
	}
	return ".csv"
}

func (f Format) contentType() string {
	if f == FormatXLSX {
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	return "text/csv; charset=utf-8"
}

// ValidateFilename checks that a file extension matches the chosen format.
func ValidateFilename(name string, format Format) error {
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(name)))
	if extension != format.extension() {
		return fmt.Errorf("selected %s format requires a %s file", strings.ToUpper(string(format)), format.extension())
	}
	return nil
}

// OpenUpload opens and validates the uploaded tabular file from a request.
func OpenUpload(r *http.Request) (Format, multipart.File, *multipart.FileHeader, error) {
	format, err := ParseFormat(r.FormValue("format"))
	if err != nil {
		return "", nil, nil, err
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		// Keep accepting the former field name for old forms and scripts.
		file, header, err = r.FormFile("csv")
	}
	if err != nil {
		return "", nil, nil, errors.New("data file required")
	}
	if err := ValidateFilename(header.Filename, format); err != nil {
		_ = file.Close()
		return "", nil, nil, err
	}
	return format, file, header, nil
}

// RowReader streams rows from a supported tabular file.
type RowReader interface {
	Read() ([]string, error)
	Close() error
}

type csvTabularReader struct {
	reader *csv.Reader
}

func (r *csvTabularReader) Read() ([]string, error) {
	return r.reader.Read()
}

func (r *csvTabularReader) Close() error {
	return nil
}

type xlsxTabularReader struct {
	workbook *excelize.File
	rows     *excelize.Rows
}

func (r *xlsxTabularReader) Read() ([]string, error) {
	if !r.rows.Next() {
		if err := r.rows.Error(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	return r.rows.Columns()
}

func (r *xlsxTabularReader) Close() error {
	return errors.Join(r.rows.Close(), r.workbook.Close())
}

// NewRowReader opens source using format.
func NewRowReader(format Format, source io.Reader) (RowReader, error) {
	switch format {
	case FormatCSV:
		buffered := bufio.NewReader(source)
		if prefix, _ := buffered.Peek(3); bytes.Equal(prefix, []byte{0xEF, 0xBB, 0xBF}) {
			if _, err := buffered.Discard(3); err != nil {
				return nil, err
			}
		}
		return &csvTabularReader{reader: csv.NewReader(buffered)}, nil
	case FormatXLSX:
		workbook, err := excelize.OpenReader(source, excelize.Options{
			UnzipSizeLimit:    maxWorkbookUncompressedSize,
			UnzipXMLSizeLimit: maxWorkbookXMLMemory,
		})
		if err != nil {
			return nil, fmt.Errorf("open Excel workbook: %w", err)
		}
		sheets := workbook.GetSheetList()
		if len(sheets) != 1 {
			_ = workbook.Close()
			return nil, fmt.Errorf("excel workbook must contain exactly one worksheet; found %d", len(sheets))
		}
		rows, err := workbook.Rows(sheets[0])
		if err != nil {
			_ = workbook.Close()
			return nil, fmt.Errorf("read worksheet %q: %w", sheets[0], err)
		}
		return &xlsxTabularReader{workbook: workbook, rows: rows}, nil
	default:
		return nil, fmt.Errorf("unsupported data format %q", format)
	}
}

// NormalizeRecord pads Excel rows whose trailing empty cells were omitted.
func NormalizeRecord(format Format, record []string, width int) []string {
	if format != FormatXLSX || len(record) >= width {
		return record
	}
	normalized := make([]string, width)
	copy(normalized, record)
	return normalized
}

// ValidateHeader checks a file header against the expected ordered columns.
func ValidateHeader(header, expected []string) error {
	if len(header) != len(expected) {
		return fmt.Errorf("header must contain %d columns; found %d", len(expected), len(header))
	}
	for index, column := range header {
		if strings.TrimSpace(column) != expected[index] {
			return fmt.Errorf("column %d must be %q; found %q", index+1, expected[index], column)
		}
	}
	return nil
}

// ReadAllRows reads an entire tabular source.
func ReadAllRows(format Format, source io.Reader) ([][]string, error) {
	reader, err := NewRowReader(format, source)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = reader.Close()
	}()

	var rows [][]string
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
}

func encodeTabularRows(format Format, sheetName string, rows [][]string) ([]byte, error) {
	switch format {
	case FormatCSV:
		var buffer bytes.Buffer
		if _, err := buffer.WriteString("\uFEFF"); err != nil {
			return nil, err
		}
		writer := csv.NewWriter(&buffer)
		if err := writer.WriteAll(rows); err != nil {
			return nil, err
		}
		return buffer.Bytes(), nil
	case FormatXLSX:
		workbook := excelize.NewFile()
		defer func() {
			_ = workbook.Close()
		}()
		if err := workbook.SetSheetName("Sheet1", sheetName); err != nil {
			return nil, err
		}
		for rowIndex, row := range rows {
			values := make([]any, len(row))
			for columnIndex, value := range row {
				values[columnIndex] = value
			}
			cell, err := excelize.CoordinatesToCellName(1, rowIndex+1)
			if err != nil {
				return nil, err
			}
			if err := workbook.SetSheetRow(sheetName, cell, &values); err != nil {
				return nil, err
			}
		}
		buffer, err := workbook.WriteToBuffer()
		if err != nil {
			return nil, err
		}
		return buffer.Bytes(), nil
	default:
		return nil, fmt.Errorf("unsupported data format %q", format)
	}
}

// WriteDownload encodes rows and writes them as an HTTP attachment.
func WriteDownload(w http.ResponseWriter, format Format, baseName, sheetName string, rows [][]string) error {
	payload, err := encodeTabularRows(format, sheetName, rows)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", format.contentType())
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s%s\"", baseName, format.extension()))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(payload)
	return err
}
