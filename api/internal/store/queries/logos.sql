-- name: CreateLogo :one
-- Content-addressed insert. Uploading identical bytes twice returns the existing
-- row rather than a duplicate; the no-op `id = logos.id` update exists purely so
-- RETURNING fires on the conflict path (DO NOTHING would return no rows).
INSERT INTO logos (content_type, byte_size, sha256, width, height, data, created_by)
VALUES (@content_type, @byte_size, @sha256, sqlc.narg('width')::int, sqlc.narg('height')::int, @data, @created_by)
ON CONFLICT (sha256) DO UPDATE SET id = logos.id
RETURNING id, content_type, byte_size, created_at;

-- name: GetLogo :one
SELECT id, content_type, byte_size, sha256, data
FROM logos
WHERE id = @id;

-- name: DeleteUnreferencedLogos :execrows
-- Reaps logos no facility points at. The age guard avoids deleting an image that
-- was uploaded seconds ago and is about to be attached to a pin.
DELETE FROM logos l
WHERE NOT EXISTS (SELECT 1 FROM facilities f WHERE f.logo_id = l.id)
  AND l.created_at < now() - @older_than::interval;

-- name: CountUnreferencedLogos :one
SELECT count(*)::int
FROM logos l
WHERE NOT EXISTS (SELECT 1 FROM facilities f WHERE f.logo_id = l.id)
  AND l.created_at < now() - @older_than::interval;
