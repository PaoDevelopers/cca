package httpapi

import "git.sr.ht/~runxiyu/cca/internal/transfer"

type tabularFormat = transfer.Format

const (
	tabularFormatCSV  = transfer.FormatCSV
	tabularFormatXLSX = transfer.FormatXLSX
)

var (
	parseTabularFormat     = transfer.ParseFormat
	openTabularUpload      = transfer.OpenUpload
	newTabularRowReader    = transfer.NewRowReader
	normalizeTabularRecord = transfer.NormalizeRecord
	validateTabularHeader  = transfer.ValidateHeader
	readAllTabularRows     = transfer.ReadAllRows
	writeTabularDownload   = transfer.WriteDownload
)
