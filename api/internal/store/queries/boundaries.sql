-- name: ListBoundaries :many
-- Emitted as GeoJSON so the frontend can hand it straight to L.geoJSON. The old
-- client had to swap every coordinate pair by hand because KML stores lng,lat
-- and Leaflet wants [lat, lng]; GeoJSON removes that step entirely.
SELECT
  b.id,
  b.area_key,
  b.name,
  b.description,
  b.external_ref,
  ST_AsGeoJSON(b.geom)::text AS geometry
FROM boundaries b
WHERE b.deleted_at IS NULL
  AND (sqlc.narg('area_key')::text IS NULL OR b.area_key = sqlc.narg('area_key')::text)
ORDER BY b.area_key, b.name;

-- name: UpsertBoundary :one
-- Geometry arrives as GeoJSON, which keeps ring-order and hole handling in one
-- well-specified format rather than in hand-built WKT.
--
-- ST_Multi coerces a single polygon into the MultiPolygon the column requires.
--
-- ST_MakeValid is NOT precautionary — it is load-bearing for the current data.
-- The Jakarta Garden City outline is genuinely invalid at source: it
-- self-intersects at 106.951776, -6.182678. Without the repair, the
-- CHECK (ST_IsValid(geom)) constraint rejects that row and the entire import
-- fails. Measured cost of the repair on that boundary:
--
--   exterior ring   229 -> 232 points   (the intersection gets noded)
--   area            3 978 744 -> 3 978 742 m²   (0.00005%)
--   Hausdorff dist  0.000000000000°     (geometrically identical)
--
-- So the outline does not move; it only becomes well-formed. Expect PostGIS ring
-- counts of JGC 232 / Metland 134 / KHI 49 / Sedayu 31, while the parser's own
-- golden test asserts the pre-repair 229 for JGC. Both numbers are correct.
--
-- A repair that yields a GeometryCollection still fails loudly, because ST_Multi
-- cannot coerce one — which is the intended behaviour for genuinely broken input.
INSERT INTO boundaries (import_key, area_key, name, description, external_ref, geom)
VALUES (
  @import_key, @area_key, @name, @description, @external_ref,
  ST_Multi(ST_MakeValid(ST_SetSRID(ST_GeomFromGeoJSON(@geometry::text), 4326)))
)
ON CONFLICT (import_key) DO UPDATE SET
  area_key     = EXCLUDED.area_key,
  name         = EXCLUDED.name,
  description  = EXCLUDED.description,
  external_ref = EXCLUDED.external_ref,
  geom         = EXCLUDED.geom,
  deleted_at   = NULL
WHERE
  boundaries.area_key IS DISTINCT FROM EXCLUDED.area_key
  OR boundaries.name IS DISTINCT FROM EXCLUDED.name
  OR boundaries.description IS DISTINCT FROM EXCLUDED.description
  OR boundaries.external_ref IS DISTINCT FROM EXCLUDED.external_ref
  OR NOT ST_Equals(boundaries.geom, EXCLUDED.geom)
  OR boundaries.deleted_at IS NOT NULL
RETURNING id, (xmax = 0)::boolean AS inserted;

-- name: SweepOrphanedBoundaries :execrows
UPDATE boundaries
SET deleted_at = now()
WHERE deleted_at IS NULL
  AND import_key <> ALL (@keep_keys::text[]);

-- name: CountBoundaries :one
SELECT count(*)::int FROM boundaries WHERE deleted_at IS NULL;
