// Package migrations ships the SQL the core applies at boot inside the
// binary, so no image layout can leave it behind.
package migrations

import "embed"

//go:embed postgres/*.sql
var FS embed.FS
