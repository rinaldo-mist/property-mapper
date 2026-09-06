package httpx

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/rinaldo-mist/property-mapper/api/internal/logging"
)

// RequestedWithHeader must accompany every state-changing request.
//
// This is the CSRF defence. SameSite=Lax already blocks cross-site cookies on
// JSON endpoints, but POST /import is multipart/form-data — a CORS-*simple*
// content type, and therefore reachable by a plain cross-site <form> submission
// that Lax alone does not stop. Requiring a custom header forces a preflight,
// which a simple form cannot send.
const (
	RequestedWithHeader = "X-Requested-With"
	RequestedWithValue  = "pm-admin"
)

// Logger logs one line per request and attaches a request-scoped logger.
func Logger(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			log := base.With("requestId", middleware.GetReqID(r.Context()))
			r = r.WithContext(logging.WithLogger(r.Context(), log))

			defer func() {
				// Health checks would otherwise dominate the log at Cloud Run's
				// probe frequency.
				level := slog.LevelInfo
				if r.URL.Path == "/api/v1/health" || r.URL.Path == "/api/v1/ready" {
					level = slog.LevelDebug
				}
				log.Log(r.Context(), level, "request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"bytes", ww.BytesWritten(),
					"durationMs", time.Since(start).Milliseconds(),
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// Recoverer converts a panic into the standard error envelope.
//
// It is mounted INSIDE Logger so the deferred log line above still records the
// 500 this produces, rather than the request vanishing from the log entirely.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec) // the server's own signal; must not be swallowed
				}
				logging.FromContext(r.Context()).Error("panic recovered",
					"panic", rec, "path", r.URL.Path, "stack", stack())
				Fail(w, r, Internal(nil))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders sets defensive headers on every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// RequireRequestedWith rejects state-changing requests without the custom header.
func RequireRequestedWith(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get(RequestedWithHeader) != RequestedWithValue {
			Fail(w, r, &APIError{
				Status:  http.StatusForbidden,
				Code:    CodeForbidden,
				Message: "Permintaan ditolak.",
			})
			return
		}
		// Sec-Fetch-Site is set by the browser and cannot be forged by page
		// script. "cross-site" on a mutating route is a CSRF attempt.
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			Fail(w, r, Forbidden())
			return
		}
		next.ServeHTTP(w, r)
	})
}
