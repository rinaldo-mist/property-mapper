package main

import (
	"context"
	"errors"
)

// Placeholders so `pmctl migrate` is usable while the import, admin and logo
// subcommands are still being built. Each is replaced by its own file.

var errNotImplemented = errors.New("not implemented yet")

func runLogos(context.Context, []string) error { return errNotImplemented }
