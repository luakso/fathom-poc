// Package sonarweb embeds the static Sonar dashboard. The publisher's emit
// step writes these files into the artifact directory alongside the JSON, so
// the pages and their data ship atomically and Caddy serves one directory.
package sonarweb

import (
	"embed"
	"io/fs"
)

// app holds the hand-authored dashboard, served as-is (no build step). all:
// keeps every file, including any future dot-prefixed assets.
//
//go:embed all:app
var app embed.FS

// Assets returns the dashboard rooted at app/.
func Assets() fs.FS {
	sub, err := fs.Sub(app, "app")
	if err != nil {
		panic(err)
	}
	return sub
}
