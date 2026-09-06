package main

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/pressly/goose/v3"

	"github.com/rinaldo-mist/property-mapper/api/db"
	"github.com/rinaldo-mist/property-mapper/api/internal/config"
)

func runMigrate(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errUsage
	}

	dsn, err := config.LoadDatabaseURL()
	if err != nil {
		return err
	}

	// goose speaks database/sql, so pgx is used through its stdlib driver rather
	// than the pool. Migrations are short-lived and single-connection.
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer sqlDB.Close()

	goose.SetBaseFS(db.Migrations)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	switch args[0] {
	case "up":
		if err := goose.UpContext(ctx, sqlDB, db.MigrationsDir); err != nil {
			return fmt.Errorf("migrate up: %w", err)
		}
		return reportVersion(ctx, sqlDB, "migrated up to")
	case "down":
		if err := goose.DownContext(ctx, sqlDB, db.MigrationsDir); err != nil {
			return fmt.Errorf("migrate down: %w", err)
		}
		return reportVersion(ctx, sqlDB, "rolled back to")
	case "status":
		goose.SetLogger(goose.NopLogger())
		migrations, err := goose.CollectMigrations(db.MigrationsDir, 0, goose.MaxVersion)
		if err != nil {
			return err
		}
		current, err := goose.GetDBVersionContext(ctx, sqlDB)
		if err != nil {
			return err
		}
		for _, m := range migrations {
			state := "pending"
			if m.Version <= current {
				state = "applied"
			}
			fmt.Printf("%-8s %5d  %s\n", state, m.Version, m.Source)
		}
		return nil
	default:
		return fmt.Errorf("unknown migrate subcommand %q (want up|down|status)", args[0])
	}
}

func reportVersion(ctx context.Context, sqlDB *sql.DB, verb string) error {
	v, err := goose.GetDBVersionContext(ctx, sqlDB)
	if err != nil {
		return err
	}
	fmt.Printf("%s schema version %d\n", verb, v)
	return nil
}
