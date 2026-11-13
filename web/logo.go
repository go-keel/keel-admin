package web

import (
	"embed"
	"io/fs"
	"path"
)

// The default logo is embedded separately from the SPA build and without the embed_ui constraint, so `go run` in dev
// serves the brand logo just like an embedded deployment: /v1/sys/brand/logo behaves the same in both forms. The glob
// matches logo.* under public/: a derived repo swaps the logo by swapping the file, not the code (a build-time assembly
// seam). With no logo.* under public/ the build fails, on purpose: the endpoint must always have a fallback asset.
//
//go:embed public/logo.*
var logoFS embed.FS

// Logo returns the built-in brand logo shipped with the binary.
// name is the file name with extension (logo.svg / logo.png), so the caller can set Content-Type; when both exist, svg
// wins (a vector stays sharp in both the favicon and the sidebar).
func Logo() (name string, data []byte) {
	names, _ := fs.Glob(logoFS, "public/logo.*")
	pick := ""
	for _, n := range names {
		if pick == "" || path.Ext(n) == ".svg" {
			pick = n
		}
	}
	data, _ = logoFS.ReadFile(pick)
	return path.Base(pick), data
}
