// Package db embeds the SQL migrations so the binary carries its own schema and
// a Cloud Run Job can migrate without a filesystem or a copied directory.
package db

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS

// MigrationsDir is the path within Migrations that goose should read.
const MigrationsDir = "migrations"
