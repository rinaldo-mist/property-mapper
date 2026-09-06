package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	issuer = "property-mapper"
	// CookieName is the session cookie. Set HttpOnly + SameSite=Lax; because the
	// Next.js rewrite proxy makes the API same-origin with the app, Lax is
	// correct and no cross-site cookie relaxation is needed.
	CookieName = "pm_session"
)

var (
	ErrTokenInvalid = errors.New("session token is invalid")
	ErrTokenExpired = errors.New("session token has expired")
)

// Claims is the session payload.
//
// Version mirrors users.token_version. Comparing the two on every request is
// what gives a stateless JWT a revocation story: bumping the column invalidates
// every token already issued to that user, without a sessions table.
type Claims struct {
	jwt.RegisteredClaims
	Version int32 `json:"ver"`
}

// Issuer mints and validates session tokens.
type Issuer struct {
	secret []byte
	ttl    time.Duration
}

func NewIssuer(secret []byte, ttl time.Duration) *Issuer {
	return &Issuer{secret: secret, ttl: ttl}
}

func (i *Issuer) TTL() time.Duration { return i.ttl }

// Issue returns a signed token for a user.
func (i *Issuer) Issue(userID string, version int32, now time.Time) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
		},
		Version: version,
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// Parse validates a token's signature, expiry and issuer.
//
// It does NOT check token_version — that requires a database read and is done by
// the middleware, so this stays a pure function.
func (i *Issuer) Parse(raw string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		// Pinning the method is what stops an "alg: none" or RS256-confusion
		// token from being accepted with the HMAC secret as a public key.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return i.secret, nil
	}, jwt.WithIssuer(issuer), jwt.WithExpirationRequired())
	switch {
	case errors.Is(err, jwt.ErrTokenExpired):
		return nil, ErrTokenExpired
	case err != nil:
		return nil, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
	case claims.Subject == "":
		return nil, ErrTokenInvalid
	}
	return claims, nil
}
