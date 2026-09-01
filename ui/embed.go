package ui

import "embed"

// dist holds both React pages, portal/ and student/, over one shared
// assets/. admin/dist is the Svelte panel's own build.
//
//go:embed all:dist all:admin/dist
var Dist embed.FS
