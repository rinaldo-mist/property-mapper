package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/time/rate"

	"github.com/rinaldo-mist/property-mapper/api/internal/auth"
	"github.com/rinaldo-mist/property-mapper/api/internal/httpx"
	"github.com/rinaldo-mist/property-mapper/api/internal/store/gen"
)

type userCtxKey struct{}

// currentUser returns the authenticated user, if RequireAdmin ran.
func currentUser(ctx context.Context) (gen.GetUserByIDRow, bool) {
	u, ok := ctx.Value(userCtxKey{}).(gen.GetUserByIDRow)
	return u, ok
}

// RequireAdmin authenticates the session cookie.
//
// Two checks beyond signature validation matter: the user must still be active,
// and the token's `ver` claim must still match users.token_version. The latter is
// what lets a password reset revoke every outstanding session without a server
// -side session store.
func (s *Server) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := s.authenticate(r)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey{}, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) authenticate(r *http.Request) (gen.GetUserByIDRow, error) {
	var zero gen.GetUserByIDRow

	cookie, err := r.Cookie(auth.CookieName)
	if err != nil || cookie.Value == "" {
		return zero, httpx.Unauthorized()
	}

	claims, err := s.issuer.Parse(cookie.Value)
	if err != nil {
		return zero, httpx.Unauthorized()
	}

	id, err := parseUUID(claims.Subject)
	if err != nil {
		return zero, httpx.Unauthorized()
	}

	user, err := s.q.GetUserByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, httpx.Unauthorized()
	}
	if err != nil {
		return zero, httpx.Internal(err)
	}
	if !user.IsActive || user.TokenVersion != claims.Version {
		return zero, httpx.Unauthorized()
	}
	return user, nil
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) error {
	var req loginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Email == "" || req.Password == "" {
		return httpx.Invalid(map[string]string{
			"email":    "Email wajib diisi.",
			"password": "Kata sandi wajib diisi.",
		})
	}

	// Limited on IP and email together: one attacker cannot lock out a real user
	// by hammering their address from elsewhere, and one IP cannot spray many
	// addresses.
	if !s.limiter.allow(clientIP(r) + "|" + req.Email) {
		return httpx.RateLimited()
	}

	user, err := s.q.GetUserByEmail(r.Context(), req.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		// Burn a hash anyway so an unknown address is not detectable by how fast
		// the request comes back.
		_, _ = auth.VerifyPassword(req.Password, dummyHash)
		return invalidCredentials()
	}
	if err != nil {
		return httpx.Internal(err)
	}

	ok, err := auth.VerifyPassword(req.Password, user.PasswordHash)
	if err != nil {
		return httpx.Internal(err)
	}
	if !ok || !user.IsActive {
		return invalidCredentials()
	}

	token, err := s.issuer.Issue(uuidString(user.ID), user.TokenVersion, time.Now())
	if err != nil {
		return httpx.Internal(err)
	}
	http.SetCookie(w, s.sessionCookie(token, s.issuer.TTL()))

	if err := s.q.TouchLastLogin(r.Context(), user.ID); err != nil {
		// Not worth failing a successful login over.
		s.log.Warn("touch last_login_at", "error", err)
	}

	httpx.NoContent(w)
	return nil
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) error {
	http.SetCookie(w, s.sessionCookie("", -time.Hour))
	httpx.NoContent(w)
	return nil
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) error {
	user, err := s.authenticate(r)
	if err != nil {
		return err
	}
	httpx.JSON(w, r, http.StatusOK, SessionUser{
		ID:          uuidString(user.ID),
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Role:        user.Role,
	})
	return nil
}

func (s *Server) sessionCookie(value string, maxAge time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     auth.CookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		// Lax is correct because the Next.js rewrite proxy makes the API
		// same-origin with the app. A cross-domain deployment would need None,
		// which would also require a real CSRF token rather than a header check.
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(maxAge.Seconds()),
	}
}

// invalidCredentials is deliberately identical for unknown email, wrong password
// and disabled account, so the endpoint cannot be used to enumerate users.
func invalidCredentials() error {
	return &httpx.APIError{
		Status:  http.StatusUnauthorized,
		Code:    httpx.CodeUnauthorized,
		Message: "Email atau kata sandi salah.",
	}
}

// dummyHash is a real Argon2id hash of an unguessable value, used to equalise
// timing on the unknown-email path.
const dummyHash = "$argon2id$v=19$m=65536,t=3,p=2$c29tZXNhbHRzb21lc2FsdA$RdescudvJCsgt3ub+b+dWRWJTmaaJObG"

// rateLimiter is an in-memory token bucket per key. In-memory is adequate:
// Cloud Run runs at most a few instances here, and a shared store would add a
// dependency for a login endpoint that sees a handful of requests a day.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateEntry
}

type rateEntry struct {
	limiter *rate.Limiter
	seen    time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{buckets: map[string]*rateEntry{}}
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	// Opportunistic sweep so the map cannot grow without bound.
	if len(l.buckets) > 1024 {
		for k, e := range l.buckets {
			if now.Sub(e.seen) > time.Hour {
				delete(l.buckets, k)
			}
		}
	}

	entry, ok := l.buckets[key]
	if !ok {
		// Five attempts up front, then one more every twelve seconds.
		entry = &rateEntry{limiter: rate.NewLimiter(rate.Every(12*time.Second), 5)}
		l.buckets[key] = entry
	}
	entry.seen = now
	return entry.limiter.Allow()
}

func clientIP(r *http.Request) string {
	// chi's RealIP middleware has already normalised X-Forwarded-For.
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	b := id.Bytes
	const hex = "0123456789abcdef"
	out := make([]byte, 36)
	for i, j := 0, 0; i < 16; i++ {
		if j == 8 || j == 13 || j == 18 || j == 23 {
			out[j] = '-'
			j++
		}
		out[j] = hex[b[i]>>4]
		out[j+1] = hex[b[i]&0x0f]
		j += 2
	}
	return string(out)
}

func parseUUID(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(s)
	return id, err
}
