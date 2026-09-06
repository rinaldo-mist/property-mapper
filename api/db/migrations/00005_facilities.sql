-- +goose Up

-- The one place an enum is right: a closed internal discriminator with no payload.
CREATE TYPE facility_source AS ENUM ('import', 'manual');

CREATE TABLE import_batches (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_kind text NOT NULL CHECK (source_kind IN ('cli','upload')),
  filename    text,
  file_sha256 bytea,
  file_size   int,
  status      text NOT NULL CHECK (status IN ('running','succeeded','failed')),
  features_inserted   int NOT NULL DEFAULT 0,
  features_updated    int NOT NULL DEFAULT 0,
  features_unchanged  int NOT NULL DEFAULT 0,
  features_deleted    int NOT NULL DEFAULT 0,
  boundaries_inserted  int NOT NULL DEFAULT 0,
  boundaries_updated   int NOT NULL DEFAULT 0,
  boundaries_unchanged int NOT NULL DEFAULT 0,
  boundaries_deleted   int NOT NULL DEFAULT 0,
  -- Every placemark the parser could not use, with the reason. Every parsing
  -- rule is fitted to a single 47-placemark sample, so a future export with
  -- unfamiliar folder names must fail loudly rather than import silently short.
  dropped     jsonb NOT NULL DEFAULT '[]'::jsonb,
  error       text,
  started_by  uuid REFERENCES users(id) ON DELETE SET NULL,
  started_at  timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz
);
CREATE INDEX import_batches_started_at_idx ON import_batches (started_at DESC);

-- One table with a source discriminator, not two.
--
-- The UI has always treated imported facilities and hand-placed pins as a single
-- list, every query path (area, category, bbox, name) is identical for both, and
-- ORDER BY name across a UNION is meaningfully worse than across one table. The
-- only real objection — that an import must never wipe manual pins — is a
-- one-line predicate, enforced structurally by the partial unique index below and
-- by the orphan sweep carrying the same WHERE source = 'import'.
CREATE TABLE facilities (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source       facility_source NOT NULL,
  -- Stable natural key, "<area-slug>:<raw-name-slug>". Deliberately excludes the
  -- category: if it did not, removing an override would change the key, turning
  -- an update into a delete-and-recreate and destroying the attached logo.
  import_key   text,
  name         text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
  area_key     text NOT NULL REFERENCES areas(key)      ON UPDATE CASCADE,
  category_key text NOT NULL REFERENCES categories(key) ON UPDATE CASCADE,
  geom         geometry(Point, 4326) NOT NULL,
  logo_id      uuid REFERENCES logos(id) ON DELETE SET NULL,

  -- Provenance, import rows only. raw_name and raw_category are what the KMZ
  -- actually said, before any override — this is how you see that "SIS JGC" was
  -- filed under Gas Station.
  source_folder    text,
  raw_name         text,
  raw_category     text,
  override_applied boolean NOT NULL DEFAULT false,

  created_by uuid REFERENCES users(id) ON DELETE SET NULL,
  updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,

  CONSTRAINT facilities_import_key_required CHECK ((source = 'import') = (import_key IS NOT NULL)),
  CONSTRAINT facilities_geom_bounds CHECK (
    ST_X(geom) BETWEEN -180 AND 180 AND ST_Y(geom) BETWEEN -90 AND 90)
);

-- Scoped to source='import' so a manual pin can never collide with an imported
-- one. Note the deliberate absence of "AND deleted_at IS NULL": a facility that
-- disappears from one export and returns in the next must be resurrected — with
-- its logo — rather than duplicated.
CREATE UNIQUE INDEX facilities_import_key_uk
  ON facilities (import_key) WHERE source = 'import';

CREATE INDEX facilities_geom_gix   ON facilities USING GIST (geom);
CREATE INDEX facilities_filter_idx ON facilities (area_key, category_key) WHERE deleted_at IS NULL;
CREATE INDEX facilities_name_trgm  ON facilities USING GIN (name gin_trgm_ops);
CREATE INDEX facilities_logo_idx   ON facilities (logo_id) WHERE logo_id IS NOT NULL;

-- MultiPolygon even though all four boundaries are single rings today: a township
-- that later covers two disjoint parcels would hard-fail a geometry(Polygon)
-- column, and ST_Multi() on insert costs nothing now versus a data migration later.
CREATE TABLE boundaries (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  import_key   text NOT NULL,
  area_key     text NOT NULL REFERENCES areas(key) ON UPDATE CASCADE,
  name         text NOT NULL,
  description  text,
  external_ref text,          -- KFMap object ID, parsed out of the description
  geom         geometry(MultiPolygon, 4326) NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz,
  CONSTRAINT boundaries_geom_valid CHECK (ST_IsValid(geom))
);
CREATE UNIQUE INDEX boundaries_import_key_uk ON boundaries (import_key);
CREATE INDEX boundaries_geom_gix ON boundaries USING GIST (geom);
CREATE INDEX boundaries_area_idx ON boundaries (area_key) WHERE deleted_at IS NULL;

CREATE TRIGGER facilities_set_updated_at
  BEFORE UPDATE ON facilities FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER boundaries_set_updated_at
  BEFORE UPDATE ON boundaries FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS boundaries;
DROP TABLE IF EXISTS facilities;
DROP TABLE IF EXISTS import_batches;
DROP TYPE IF EXISTS facility_source;
