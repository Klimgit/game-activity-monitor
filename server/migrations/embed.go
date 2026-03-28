package migrations

import "embed"

// FS holds the embedded SQL migration files.
// golang-migrate reads them via the iofs source driver.
//
//go:embed *.sql
var FS embed.FS
