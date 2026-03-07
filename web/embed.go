// Package web provides embedded frontend static files.
package web

import "embed"

//go:embed all:out
var StaticFS embed.FS
