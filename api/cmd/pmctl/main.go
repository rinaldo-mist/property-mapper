// Command pmctl is the operator CLI: migrations, KMZ imports, and admin users.
//
// It ships in the same image as the server and runs in production as a Cloud Run
// Job. Migrations are deliberately never run on container start — a service that
// scales out would race itself, which is a data-loss shape.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

const usage = `pmctl — property-mapper operator CLI

Usage:
  pmctl migrate up             Apply all pending migrations
  pmctl migrate down           Roll back the most recent migration
  pmctl migrate status         Show which migrations have been applied
  pmctl import --file <kmz>    Import a KMZ (idempotent; safe to re-run)
  pmctl admin create --email <email> [--name <name>]
                               Create an admin user (prompts for a password)
  pmctl logos gc [--dry-run]   Delete logos no facility references

Environment:
  DATABASE_URL   required by every subcommand
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		if errors.Is(err, errUsage) {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "pmctl: %v\n", err)
		os.Exit(1)
	}
}

var errUsage = errors.New("usage")

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errUsage
	}
	switch args[0] {
	case "migrate":
		return runMigrate(ctx, args[1:])
	case "import":
		return runImport(ctx, args[1:])
	case "admin":
		return runAdmin(ctx, args[1:])
	case "logos":
		return runLogos(ctx, args[1:])
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
	}
}
