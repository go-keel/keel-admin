//go:build !embed_ui

package web

import "io/fs"

// FS returns nil when not compiled with -tags embed_ui (dev: use vite dev server).
func FS() fs.FS { return nil }
