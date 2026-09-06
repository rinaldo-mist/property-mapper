package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/term"

	"github.com/rinaldo-mist/property-mapper/api/internal/auth"
	"github.com/rinaldo-mist/property-mapper/api/internal/config"
	"github.com/rinaldo-mist/property-mapper/api/internal/store"
	"github.com/rinaldo-mist/property-mapper/api/internal/store/gen"
)

func runAdmin(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errUsage
	}
	switch args[0] {
	case "create":
		return adminCreate(ctx, args[1:], false)
	case "set-password":
		return adminCreate(ctx, args[1:], true)
	default:
		return fmt.Errorf("unknown admin subcommand %q (want create|set-password)", args[0])
	}
}

func adminCreate(ctx context.Context, args []string, reset bool) error {
	fs := flag.NewFlagSet("admin", flag.ContinueOnError)
	email := fs.String("email", "", "admin email address (required)")
	name := fs.String("name", "", "display name (optional)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*email) == "" {
		return errors.New("--email is required")
	}

	dsn, err := config.LoadDatabaseURL()
	if err != nil {
		return err
	}

	password, err := readPassword()
	if err != nil {
		return err
	}
	if err := auth.ValidatePassword(password); err != nil {
		return err
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	pool, err := store.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	q := gen.New(pool)

	if reset {
		rows, err := q.SetUserPassword(ctx, gen.SetUserPasswordParams{
			Email:        *email,
			PasswordHash: hash,
		})
		if err != nil {
			return fmt.Errorf("set password: %w", err)
		}
		if rows == 0 {
			return fmt.Errorf("no user with email %s", *email)
		}
		// SetUserPassword also bumps token_version, so every session issued under
		// the old password stops working immediately.
		fmt.Printf("password updated for %s; all existing sessions revoked\n", *email)
		return nil
	}

	user, err := q.CreateUser(ctx, gen.CreateUserParams{
		Email:        *email,
		PasswordHash: hash,
		DisplayName:  nilIfEmpty(strings.TrimSpace(*name)),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("a user with email %s already exists (use: pmctl admin set-password --email %s)", *email, *email)
		}
		return fmt.Errorf("create user: %w", err)
	}

	fmt.Printf("created admin %s (%s)\n", user.Email, uuidString(user.ID))
	return nil
}

// readPassword takes the password from PMCTL_PASSWORD when set — which is how a
// Cloud Run Job supplies it from Secret Manager — and otherwise prompts without
// echoing. Passing it as a flag is deliberately not supported: command lines end
// up in shell history and process listings.
func readPassword() (string, error) {
	if fromEnv := os.Getenv("PMCTL_PASSWORD"); fromEnv != "" {
		return fromEnv, nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("no terminal available; set PMCTL_PASSWORD instead")
	}

	fmt.Fprint(os.Stderr, "Password: ")
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}

	fmt.Fprint(os.Stderr, "Confirm:  ")
	second, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}

	if string(first) != string(second) {
		return "", errors.New("passwords do not match")
	}
	return string(first), nil
}

// isUniqueViolation reports whether err is a Postgres 23505.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	b := id.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
