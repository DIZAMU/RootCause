//go:build webdist

// Package web holds the built dashboard (ARCHITECTURE.md section 11). With the build tag
// webdist, the binary embeds web/dist, which scripts/build-web.sh creates.
// Without the tag, a fresh clone builds with no Node toolchain, and the
// dashboard serves a page that says how to build it.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Assets returns the built dashboard: index.html and assets/.
func Assets() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // "dist" is a valid path, so fs.Sub cannot fail
	}
	return sub
}
