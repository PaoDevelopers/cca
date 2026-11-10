package ui

import "embed"

// dist holds the React pages, portal/, student/ and admin/, over one
// shared assets/.
//
//go:embed all:dist
var Dist embed.FS
