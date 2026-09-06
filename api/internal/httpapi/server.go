// Package httpapi declares every route in one place and implements the handlers.
//
// Router() is the single entrypoint: cmd/server mounts it to serve traffic, and
// tests mount it directly through httptest. Nothing about the deployment target
// leaks in here, so moving off Cloud Run touches the Dockerfile and CI rather
// than the application.
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rinaldo-mist/property-mapper/api/internal/auth"
	"github.com/rinaldo-mist/property-mapper/api/internal/config"
	"github.com/rinaldo-mist/property-mapper/api/internal/httpx"
	"github.com/rinaldo-mist/property-mapper/api/internal/importing"
	"github.com/rinaldo-mist/property-mapper/api/internal/store/gen"
)

type Server struct {
	cfg      config.Config
	log      *slog.Logger
	pool     *pgxpool.Pool
	q        *gen.Queries
	issuer   *auth.Issuer
	importer *importing.Service
	limiter  *rateLimiter
}

func NewServer(cfg config.Config, log *slog.Logger, pool *pgxpool.Pool) *Server {
	return &Server{
		cfg:      cfg,
		log:      log,
		pool:     pool,
		q:        gen.New(pool),
		issuer:   auth.NewIssuer(cfg.JWTSecret, cfg.SessionTTL),
		importer: importing.New(pool),
		limiter:  newRateLimiter(),
	}
}

// Router builds the complete route table.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(httpx.SecurityHeaders)
	if len(s.cfg.CORSAllowedOrigins) > 0 {
		// Normally empty: the browser reaches this API same-origin through the
		// Next.js rewrite proxy, so CORS never enters the picture.
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   s.cfg.CORSAllowedOrigins,
			AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Content-Type", httpx.RequestedWithHeader},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}
	r.Use(httpx.Logger(s.log))
	r.Use(httpx.Recoverer)

	r.Route("/api/v1", func(r chi.Router) {
		// Applied to the whole API rather than per-route. It is a no-op on safe
		// methods, so mounting it once here makes it structurally impossible to
		// ship a mutating endpoint without CSRF protection.
		r.Use(httpx.RequireRequestedWith)

		r.Get("/health", handle(s.handleHealth))
		r.Get("/ready", handle(s.handleReady))

		r.Get("/catalog", handle(s.handleCatalog))
		r.Get("/facilities", handle(s.handleListFacilities))
		r.Get("/boundaries", handle(s.handleListBoundaries))
		r.Get("/map", handle(s.handleMap))
		r.Get("/logos/{id}", handle(s.handleGetLogo))

		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", handle(s.handleLogin))
			r.Post("/logout", handle(s.handleLogout))
			r.Get("/me", handle(s.handleMe))
		})

		// Everything below requires a valid admin session.
		r.Group(func(r chi.Router) {
			r.Use(s.RequireAdmin)

			r.Post("/pins", handle(s.handleCreatePin))
			r.Patch("/pins/{id}", handle(s.handleUpdatePin))
			r.Delete("/pins/{id}", handle(s.handleDeletePin))

			r.Post("/logos", handle(s.handleUploadLogo))

			r.Post("/import", handle(s.handleImport))
			r.Get("/import/batches", handle(s.handleListImportBatches))
		})
	})

	r.NotFound(handle(func(http.ResponseWriter, *http.Request) error {
		return httpx.NotFound()
	}))
	r.MethodNotAllowed(handle(func(http.ResponseWriter, *http.Request) error {
		return &httpx.APIError{
			Status:  http.StatusMethodNotAllowed,
			Code:    httpx.CodeBadRequest,
			Message: "Metode tidak diizinkan.",
		}
	}))

	return r
}

// handle adapts an error-returning handler to http.HandlerFunc, so handlers can
// `return err` instead of remembering to write a response and return.
func handle(fn func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			httpx.Fail(w, r, err)
		}
	}
}
