-- name: ListFacilities :many
-- Every filter is optional: a NULL parameter disables its clause, so one query
-- serves /map (all filters null) and /facilities (any subset).
--
-- count(*) OVER () yields the pre-LIMIT total, avoiding a second round trip and
-- the duplicated WHERE clause a separate count query would need.
SELECT
  f.id,
  f.source,
  f.name,
  f.area_key,
  f.category_key,
  ST_X(f.geom)::float8 AS lng,
  ST_Y(f.geom)::float8 AS lat,
  f.logo_id,
  f.updated_at,
  count(*) OVER ()::int AS total_count
FROM facilities f
WHERE f.deleted_at IS NULL
  AND (sqlc.narg('area_key')::text IS NULL OR f.area_key = sqlc.narg('area_key')::text)
  AND (cardinality(@category_keys::text[]) = 0 OR f.category_key = ANY(@category_keys::text[]))
  AND (sqlc.narg('query')::text IS NULL OR f.name ILIKE '%' || sqlc.narg('query')::text || '%')
  AND (
    sqlc.narg('min_lng')::float8 IS NULL
    OR f.geom && ST_MakeEnvelope(
         sqlc.narg('min_lng')::float8, sqlc.narg('min_lat')::float8,
         sqlc.narg('max_lng')::float8, sqlc.narg('max_lat')::float8, 4326)
  )
ORDER BY f.name, f.id
LIMIT COALESCE(sqlc.narg('limit_')::int, 5000)
OFFSET COALESCE(sqlc.narg('offset_')::int, 0);

-- name: GetFacility :one
SELECT
  f.id,
  f.source,
  f.name,
  f.area_key,
  f.category_key,
  ST_X(f.geom)::float8 AS lng,
  ST_Y(f.geom)::float8 AS lat,
  f.logo_id,
  f.updated_at
FROM facilities f
WHERE f.id = @id AND f.deleted_at IS NULL;

-- name: UpsertImportedFacility :one
-- The heart of a re-runnable import.
--
-- Three details are load-bearing:
--   * logo_id is absent from the SET list, so a logo an operator attached to an
--     imported facility survives every future import.
--   * deleted_at is reset to NULL, so a facility that disappears from one export
--     and returns in the next is resurrected — with its logo — not duplicated.
--   * The WHERE guard suppresses no-op updates, keeping updated_at honest. It
--     also means an unchanged row returns ZERO rows, which the caller reads as
--     "unchanged" rather than as an error.
INSERT INTO facilities (
  source, import_key, name, area_key, category_key, geom,
  source_folder, raw_name, raw_category, override_applied
) VALUES (
  'import', @import_key, @name, @area_key, @category_key,
  ST_SetSRID(ST_MakePoint(@lng::float8, @lat::float8), 4326),
  @source_folder, @raw_name, @raw_category, @override_applied
)
ON CONFLICT (import_key) WHERE source = 'import' DO UPDATE SET
  name             = EXCLUDED.name,
  area_key         = EXCLUDED.area_key,
  category_key     = EXCLUDED.category_key,
  geom             = EXCLUDED.geom,
  source_folder    = EXCLUDED.source_folder,
  raw_name         = EXCLUDED.raw_name,
  raw_category     = EXCLUDED.raw_category,
  override_applied = EXCLUDED.override_applied,
  deleted_at       = NULL
WHERE
  facilities.name IS DISTINCT FROM EXCLUDED.name
  OR facilities.area_key IS DISTINCT FROM EXCLUDED.area_key
  OR facilities.category_key IS DISTINCT FROM EXCLUDED.category_key
  OR facilities.source_folder IS DISTINCT FROM EXCLUDED.source_folder
  OR facilities.raw_category IS DISTINCT FROM EXCLUDED.raw_category
  OR facilities.override_applied IS DISTINCT FROM EXCLUDED.override_applied
  OR NOT ST_Equals(facilities.geom, EXCLUDED.geom)
  OR facilities.deleted_at IS NOT NULL
RETURNING id, (xmax = 0)::boolean AS inserted;

-- name: SweepOrphanedFacilities :execrows
-- Soft-deletes imported rows absent from the KMZ just processed.
--
-- Keyed on the array of surviving import keys rather than on a batch id, because
-- the upsert above deliberately reports nothing for unchanged rows — a batch-id
-- sweep would delete every facility that did not happen to change.
--
-- Callers must refuse to run this with an empty array: a parse that produced no
-- features means a broken KMZ, not an empty one.
UPDATE facilities
SET deleted_at = now()
WHERE source = 'import'
  AND deleted_at IS NULL
  AND import_key <> ALL (@keep_keys::text[]);

-- name: CreateManualPin :one
INSERT INTO facilities (
  source, name, area_key, category_key, geom, logo_id, created_by, updated_by
) VALUES (
  'manual', @name, @area_key, @category_key,
  ST_SetSRID(ST_MakePoint(@lng::float8, @lat::float8), 4326),
  sqlc.narg('logo_id')::uuid, @actor, @actor
)
RETURNING id;

-- name: UpdateManualPin :one
-- Partial update: a NULL parameter leaves its column alone.
--
-- The `source = 'manual'` predicate is what makes an admin editing an imported
-- facility a 404 rather than a silent success, preserving the old rule that only
-- hand-placed pins are editable.
UPDATE facilities SET
  name         = COALESCE(sqlc.narg('name')::text, name),
  area_key     = COALESCE(sqlc.narg('area_key')::text, area_key),
  category_key = COALESCE(sqlc.narg('category_key')::text, category_key),
  geom = CASE
    WHEN sqlc.narg('lng')::float8 IS NOT NULL AND sqlc.narg('lat')::float8 IS NOT NULL
      THEN ST_SetSRID(ST_MakePoint(sqlc.narg('lng')::float8, sqlc.narg('lat')::float8), 4326)
    ELSE geom
  END,
  -- Three-way: clear it, set it, or leave it. "Hapus logo" sends clear_logo.
  logo_id = CASE
    WHEN @clear_logo::boolean THEN NULL
    WHEN sqlc.narg('logo_id')::uuid IS NOT NULL THEN sqlc.narg('logo_id')::uuid
    ELSE logo_id
  END,
  updated_by = @actor
WHERE id = @id AND source = 'manual' AND deleted_at IS NULL
RETURNING id;

-- name: DeleteManualPin :execrows
-- Hard delete: manual pins are user content, and the pre-refactor app removed
-- them outright. Imported rows are soft-deleted instead, by the sweep above.
DELETE FROM facilities WHERE id = @id AND source = 'manual';

-- name: CountFacilitiesBySource :one
SELECT
  count(*) FILTER (WHERE source = 'import')::int AS imported,
  count(*) FILTER (WHERE source = 'manual')::int AS manual
FROM facilities
WHERE deleted_at IS NULL;
