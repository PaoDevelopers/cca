package web

import "embed"

// Embedded so that serving these does not depend on the working
// directory.

//go:embed all:static
var staticFS embed.FS
