-- name: ListAreas :many
-- facility_count drives the "N fasilitas" line under each developer button.
SELECT
  a.key,
  a.label,
  a.short_code,
  a.sort_order,
  (SELECT count(*) FROM facilities f
    WHERE f.area_key = a.key AND f.deleted_at IS NULL)::int AS facility_count
FROM areas a
ORDER BY a.sort_order, a.key;

-- name: ListCategories :many
-- facility_count exists so the UI can render chips only for categories that
-- actually occur in the data — five today — while the pin dialog's dropdown
-- offers all six. Reproducing that split is a fidelity requirement.
SELECT
  c.key,
  c.label,
  c.color,
  c.icon,
  c.sort_order,
  c.is_fallback,
  (SELECT count(*) FROM facilities f
    WHERE f.category_key = c.key AND f.deleted_at IS NULL)::int AS facility_count
FROM categories c
ORDER BY c.sort_order, c.key;

-- name: ListCategoryAliases :many
SELECT alias, category_key FROM category_aliases ORDER BY alias;

-- name: ListImportOverrides :many
SELECT match_name, set_name, set_area_key, set_category_key
FROM import_overrides
ORDER BY match_name;

-- name: GetFallbackCategoryKey :one
SELECT key FROM categories WHERE is_fallback LIMIT 1;

-- name: AreaExists :one
SELECT EXISTS (SELECT 1 FROM areas WHERE key = @key)::boolean;

-- name: CategoryExists :one
SELECT EXISTS (SELECT 1 FROM categories WHERE key = @key)::boolean;
