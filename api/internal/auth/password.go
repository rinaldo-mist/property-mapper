// Package auth holds password hashing, session tokens, and the request-scoped
// identity they resolve to.
package auth

import (
	"errors"
	"fmt"

	"github.com/alexedwards/argon2id"
)

// Argon2id parameters.
//
// 64 MiB is affordable because the API runs as a long-lived container rather
// than a memory-capped serverless function — a per-request isolate would have
// forced this down to the 19 MiB OWASP floor. Verification is deliberately
// expensive, so concurrent logins are gated by loginSlots below.
var hashParams = &argon2id.Params{
	Memory:      64 * 1024, // KiB
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// loginSlots caps how many password verifications run at once. Four concurrent
// hashes is 256 MiB of transient memory; without the cap, a burst of login
// attempts could exhaust the container's memory rather than merely queue.
var loginSlots = make(chan struct{}, 4)

// ErrInvalidCredentials is returned for every authentication failure — unknown
// email, wrong password, or disabled account — so the caller cannot distinguish
// them and enumerate valid addresses.
var ErrInvalidCredentials = errors.New("invalid credentials")

// HashPassword produces a PHC-format string that embeds the parameters and salt,
// so the cost can be raised later without invalidating existing hashes.
func HashPassword(plaintext string) (string, error) {
	loginSlots <- struct{}{}
	defer func() { <-loginSlots }()

	hash, err := argon2id.CreateHash(plaintext, hashParams)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return hash, nil
}

// VerifyPassword reports whether plaintext matches the stored PHC hash.
func VerifyPassword(plaintext, encodedHash string) (bool, error) {
	loginSlots <- struct{}{}
	defer func() { <-loginSlots }()

	match, err := argon2id.ComparePasswordAndHash(plaintext, encodedHash)
	if err != nil {
		return false, fmt.Errorf("verify password: %w", err)
	}
	return match, nil
}

// MinPasswordLength is enforced wherever a password is set. Deliberately a
// length floor rather than a character-class rule: composition requirements
// push people toward predictable substitutions without adding real entropy.
const MinPasswordLength = 12

// ValidatePassword checks a new password before it is hashed.
func ValidatePassword(plaintext string) error {
	if len([]rune(plaintext)) < MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	}
	return nil
}
