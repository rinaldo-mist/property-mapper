-- +goose Up
-- Extensions are created here rather than by a docker-entrypoint initdb script so
-- that a local PostGIS container and a fresh Neon database go through exactly the
-- same code path. One less dev/prod divergence, one less compose volume.
CREATE EXTENSION IF NOT EXISTS postgis;   -- geometry columns, GiST, ST_AsGeoJSON
CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS citext;    -- case-insensitive unique email
CREATE EXTENSION IF NOT EXISTS pg_trgm;   -- trigram index for name search

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS set_updated_at();
-- The extensions are intentionally left in place: dropping postgis would cascade
-- to every geometry column, and other databases on the cluster may rely on them.
