package ui

import "embed"

//go:embed all:student/dist all:admin/dist all:portal/dist
var Dist embed.FS
