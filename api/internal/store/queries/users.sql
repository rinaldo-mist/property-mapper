-- name: GetUserByEmail :one
-- Returns inactive users too: the handler must still burn a password-verify on
-- them so that a disabled account is not distinguishable by response time.
SELECT id, email, password_hash, display_name, role, is_active, token_version
FROM users
WHERE email = @email;

-- name: GetUserByID :one
SELECT id, email, display_name, role, is_active, token_version
FROM users
WHERE id = @id;

-- name: CreateUser :one
INSERT INTO users (email, password_hash, display_name)
VALUES (@email, @password_hash, sqlc.narg('display_name')::text)
RETURNING id, email, display_name, role, created_at;

-- name: SetUserPassword :execrows
-- Bumping token_version in the same statement invalidates every session issued
-- under the old password.
UPDATE users
SET password_hash = @password_hash,
    token_version = token_version + 1
WHERE email = @email;

-- name: TouchLastLogin :exec
UPDATE users SET last_login_at = now() WHERE id = @id;

-- name: CountUsers :one
SELECT count(*)::int FROM users;
