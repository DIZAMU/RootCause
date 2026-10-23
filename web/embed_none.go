//go:build !webdist

package web

import "io/fs"

// Assets returns nil: this binary was built without the dashboard (no
// webdist build tag).
func Assets() fs.FS { return nil }
