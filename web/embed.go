package web

import (
	"embed"
)

// Embed FS containing static assets. Paths are relative to this directory.
//
//go:embed static/dashboard.html static/style.css
var FS embed.FS
