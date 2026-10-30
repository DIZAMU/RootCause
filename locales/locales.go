// Package locales holds the interface strings (ARCHITECTURE.md section 12). They are
// embedded in the binary, so rootcause can show translated messages from any
// folder, including the error for not finding the project root.
package locales

import "embed"

// FS holds en.json and no.json.
//
//go:embed *.json
var FS embed.FS
