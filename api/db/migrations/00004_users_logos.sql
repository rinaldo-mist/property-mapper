-- +goose Up
-- Ordered before facilities because facilities.logo_id references logos(id).

CREATE TABLE users (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email         citext NOT NULL UNIQUE CHECK (position('@' in email) > 1),
  -- Full PHC string: $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
  password_hash text NOT NULL,
  display_name  text,
  role          text NOT NULL DEFAULT 'admin' CHECK (role IN ('admin')),
  is_active     boolean NOT NULL DEFAULT true,
  -- Bumping this invalidates every outstanding JWT for the user, which is how a
  -- stateless session gets a revocation story without a sessions table.
  token_version int NOT NULL DEFAULT 0,
  last_login_at timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Logos live in Postgres rather than object storage. At roughly twenty images of
-- at most 500 KB the performance argument is theoretical, and the operational one
-- is decisive: pg_dump then restores the application completely, with no second
-- artifact to back up separately and no orphaned files to reap.
CREATE TABLE logos (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  content_type text NOT NULL CHECK (content_type IN
                 ('image/png','image/jpeg','image/webp','image/svg+xml')),
  -- Mirrors the 500 KB client-side cap the old upload form enforced.
  byte_size    int NOT NULL CHECK (byte_size > 0 AND byte_size <= 500000),
  -- Content-addressed: uploading the same bytes twice reuses one row, and the
  -- bytes behind an id can never change, which is what makes the immutable
  -- Cache-Control header on GET /api/v1/logos/{id} safe.
  sha256       bytea NOT NULL UNIQUE CHECK (length(sha256) = 32),
  width        int,
  height       int,   -- both NULL for SVG
  data         bytea NOT NULL,
  created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER users_set_updated_at
  BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS logos;
DROP TABLE IF EXISTS users;
