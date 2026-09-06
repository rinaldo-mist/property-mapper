package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/rinaldo-mist/property-mapper/api/internal/config"
	"github.com/rinaldo-mist/property-mapper/api/internal/importing"
	"github.com/rinaldo-mist/property-mapper/api/internal/store"
)

func runImport(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	file := fs.String("file", "", "path to the .kmz to import (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("--file is required")
	}

	dsn, err := config.LoadDatabaseURL()
	if err != nil {
		return err
	}

	f, err := os.Open(*file)
	if err != nil {
		return fmt.Errorf("open %s: %w", *file, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	pool, err := store.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	// *os.File is an io.ReaderAt, so the CLI and the upload handler feed the very
	// same method — no second import implementation to keep in sync.
	report, err := importing.New(pool).RunImport(ctx, f, info.Size(), importing.Meta{
		SourceKind: importing.SourceCLI,
		Filename:   info.Name(),
	})
	if err != nil {
		return err
	}

	printReport(report)
	return nil
}

func printReport(r *importing.Report) {
	fmt.Printf("import %s (%s)\n", r.BatchID, r.Filename)
	fmt.Printf("  facilities   %d inserted, %d updated, %d unchanged, %d deleted\n",
		r.Features.Inserted, r.Features.Updated, r.Features.Unchanged, r.Features.Deleted)
	fmt.Printf("  boundaries   %d inserted, %d updated, %d unchanged, %d deleted\n",
		r.Boundaries.Inserted, r.Boundaries.Updated, r.Boundaries.Unchanged, r.Boundaries.Deleted)

	if len(r.Dropped) == 0 {
		return
	}
	// Loud on purpose: every parsing rule is fitted to a single sample export, so
	// a placemark the parser could not resolve is the signal that the next KMZ
	// has a shape the rules do not cover yet.
	fmt.Printf("\n  %d placemark(s) DROPPED — these are not in the database:\n", len(r.Dropped))
	for _, d := range r.Dropped {
		fmt.Printf("    - %q at %s\n      %s\n", d.Name, d.SourceFolder, d.Reason)
	}
}
