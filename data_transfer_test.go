package main

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestTabularRowsRoundTrip(t *testing.T) {
	t.Parallel()

	want := [][]string{
		{"id", "name", "notes"},
		{"23321", "Test Student", ""},
		{"CCA-1", "Basketball Team", "Monday CCA 1,Tuesday CCA 1"},
	}

	for _, format := range []tabularFormat{tabularFormatCSV, tabularFormatXLSX} {
		format := format
		t.Run(string(format), func(t *testing.T) {
			t.Parallel()
			payload, err := encodeTabularRows(format, "Students", want)
			if err != nil {
				t.Fatalf("encode rows: %v", err)
			}
			got, err := readAllTabularRows(format, bytes.NewReader(payload))
			if err != nil {
				t.Fatalf("read rows: %v", err)
			}
			for index := range got {
				got[index] = normalizeTabularRecord(format, got[index], len(want[index]))
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("round trip mismatch\ngot:  %#v\nwant: %#v", got, want)
			}
		})
	}
}

func TestExcelWorkbookRequiresOneWorksheet(t *testing.T) {
	t.Parallel()

	workbook := excelize.NewFile()
	defer func() {
		_ = workbook.Close()
	}()
	if _, err := workbook.NewSheet("Second"); err != nil {
		t.Fatalf("add worksheet: %v", err)
	}
	payload, err := workbook.WriteToBuffer()
	if err != nil {
		t.Fatalf("write workbook: %v", err)
	}
	if _, err := newTabularRowReader(tabularFormatXLSX, bytes.NewReader(payload.Bytes())); err == nil {
		t.Fatal("expected multiple worksheets to be rejected")
	}
}

func TestValidateTabularFilename(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		format  tabularFormat
		wantErr bool
	}{
		{name: "students.csv", format: tabularFormatCSV},
		{name: "students.XLSX", format: tabularFormatXLSX},
		{name: "students.xlsx", format: tabularFormatCSV, wantErr: true},
		{name: "students.xls", format: tabularFormatXLSX, wantErr: true},
	} {
		err := validateTabularFilename(test.name, test.format)
		if (err != nil) != test.wantErr {
			t.Fatalf("validate %q as %q: err=%v, wantErr=%t", test.name, test.format, err, test.wantErr)
		}
	}
}

func TestValidateTabularHeader(t *testing.T) {
	t.Parallel()

	expected := []string{"id", "name"}
	if err := validateTabularHeader([]string{"id", "name"}, expected); err != nil {
		t.Fatalf("valid header rejected: %v", err)
	}
	if err := validateTabularHeader([]string{"name", "id"}, expected); err == nil {
		t.Fatal("expected reordered header to be rejected")
	}
}
