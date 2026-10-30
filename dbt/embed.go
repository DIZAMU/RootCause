// Package dbtproject embeds the dbt project (ARCHITECTURE.md section 9.2) in the binary.
// internal/dbt extracts it into a fresh directory for each build, so a build
// never reads the working tree, where a file caught mid-save could silently
// drop models or tests from the run.
package dbtproject

import "embed"

// FS holds the project files dbt reads: the project and profile files, the
// models and the singular tests.
//
//go:embed dbt_project.yml profiles.yml models tests
var FS embed.FS
