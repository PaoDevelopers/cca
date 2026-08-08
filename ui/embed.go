package ui

import "embed"

//go:embed all:student/dist all:admin/dist
var Dist embed.FS
