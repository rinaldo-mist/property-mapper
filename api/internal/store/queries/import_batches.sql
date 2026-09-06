-- name: CreateImportBatch :one
INSERT INTO import_batches (source_kind, filename, file_sha256, file_size, status, started_by)
VALUES (@source_kind, sqlc.narg('filename')::text, @file_sha256, @file_size, 'running', sqlc.narg('started_by')::uuid)
RETURNING id, started_at;

-- name: FinishImportBatch :exec
UPDATE import_batches SET
  status = @status,
  features_inserted    = @features_inserted,
  features_updated     = @features_updated,
  features_unchanged   = @features_unchanged,
  features_deleted     = @features_deleted,
  boundaries_inserted  = @boundaries_inserted,
  boundaries_updated   = @boundaries_updated,
  boundaries_unchanged = @boundaries_unchanged,
  boundaries_deleted   = @boundaries_deleted,
  dropped              = @dropped,
  error                = sqlc.narg('error_')::text,
  finished_at          = now()
WHERE id = @id;

-- name: ListImportBatches :many
SELECT
  id, source_kind, filename, file_size, status,
  features_inserted, features_updated, features_unchanged, features_deleted,
  boundaries_inserted, boundaries_updated, boundaries_unchanged, boundaries_deleted,
  dropped, error, started_by, started_at, finished_at
FROM import_batches
ORDER BY started_at DESC
LIMIT COALESCE(sqlc.narg('limit_')::int, 20);
