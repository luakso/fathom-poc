// Package web embeds the static dashboard site. The publisher's emit step
// writes these files into the artifact directory alongside the JSON, so the
// page and its data ship atomically and Caddy serves one directory.
package web

import "embed"

// Site holds the dashboard under "site/". all: keeps every file, including
// any future dot-prefixed assets.
//
//go:embed all:site
var Site embed.FS
