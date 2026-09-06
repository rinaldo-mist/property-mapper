// Command server runs the HTTP API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rinaldo-mist/property-mapper/api/internal/config"
	"github.com/rinaldo-mist/property-mapper/api/internal/httpapi"
	"github.com/rinaldo-mist/property-mapper/api/internal/logging"
	"github.com/rinaldo-mist/property-mapper/api/internal/store"
)

// shutdownGrace is 8 seconds, not the 15 you would pick for a VPS: Cloud Run
// sends SIGTERM and then SIGKILLs 10 seconds later, so a longer budget would
// mean in-flight requests are killed rather than drained.
const shutdownGrace = 8 * time.Second

func main() {
	// -health lets the container health-check itself. The distroless base image
	// has no shell and no wget, so the binary provides the probe.
	healthCheck := flag.Bool("health", false, "probe the local server and exit 0 or 1")
	flag.Parse()

	if *healthCheck {
		os.Exit(probe())
	}
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logging.New(cfg.LogLevel, cfg.AppEnv)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	srv := &http.Server{
		Addr:    net.JoinHostPort("", cfg.Port),
		Handler: httpapi.NewServer(cfg, log, pool).Router(),

		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout is deliberately zero. A global write deadline would abort
		// a slow KMZ import mid-transaction; per-request deadlines belong in the
		// handlers, where they can be sized to the work.
		WriteTimeout: 0,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr, "env", cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
		log.Info("shutdown signal received, draining", "grace", shutdownGrace.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed; closing connections", "error", err)
		_ = srv.Close()
		return err
	}
	log.Info("shutdown complete")
	return nil
}

// probe requests the health endpoint on the configured port.
func probe() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/api/v1/health")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
