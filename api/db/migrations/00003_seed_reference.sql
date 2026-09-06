-- +goose Up
-- Seeded in a migration rather than a separate script: these rows are foreign-key
-- prerequisites for every import, and the application is meaningless without them.
-- ON CONFLICT DO UPDATE keeps the migration re-runnable and makes it the single
-- place to correct a label or colour.
--
-- Values are ported verbatim from the pre-refactor AREA_NAMES, CATEGORY_META,
-- CATEGORY_ALIASES and FEATURE_FIXES tables.

INSERT INTO areas (key, label, short_code, sort_order) VALUES
  ('Sedayu',  'Sedayu City',        'Sedayu', 1),
  ('JGC',     'Jakarta Garden City', 'JGC',   2),
  ('KHI',     'Kota Harapan Indah',  'KHI',   3),
  ('Metland', 'Metland Menteng',     'MTL',   4)
ON CONFLICT (key) DO UPDATE
  SET label = EXCLUDED.label,
      short_code = EXCLUDED.short_code,
      sort_order = EXCLUDED.sort_order;

-- Labels are the Indonesian UI strings, not the keys: 'Showroom Dealer' displays
-- as "Showroom", 'Gas Station' as "SPBU", 'Other' as "Lainnya".
INSERT INTO categories (key, label, color, icon, sort_order, is_fallback) VALUES
  ('School',          'School',     '#3f7ad8', 'S', 1, false),
  ('Hospital',        'Hospital',   '#db5457', '+', 2, false),
  ('Showroom Dealer', 'Showroom',   '#e39b32', 'D', 3, false),
  ('Gas Station',     'SPBU',       '#7657c9', 'F', 4, false),
  ('University',      'University', '#168b91', 'U', 5, false),
  ('Other',           'Lainnya',    '#68726c', '•', 6, true)
ON CONFLICT (key) DO UPDATE
  SET label = EXCLUDED.label,
      color = EXCLUDED.color,
      icon = EXCLUDED.icon,
      sort_order = EXCLUDED.sort_order,
      is_fallback = EXCLUDED.is_fallback;

INSERT INTO category_aliases (alias, category_key) VALUES
  ('Sekolah',         'School'),          -- the only genuinely Indonesian folder name
  ('School',          'School'),
  ('Hospital',        'Hospital'),
  ('Showroom Dealer', 'Showroom Dealer'),
  ('Gas Station',     'Gas Station'),
  ('University',      'University')
ON CONFLICT (alias) DO UPDATE SET category_key = EXCLUDED.category_key;

INSERT INTO import_overrides (match_name, set_name, set_area_key, set_category_key, note) VALUES
  ('SIS JGC', NULL, 'JGC', 'School',
   'Filed under JGC/Gas Station in the export. It is a school; only the category is wrong.'),
  ('Suzuki', 'Suzuki Sedayu', 'Sedayu', 'Showroom Dealer',
   'Lone placemark under the root folder with no area or category context.')
ON CONFLICT (match_name) DO UPDATE
  SET set_name = EXCLUDED.set_name,
      set_area_key = EXCLUDED.set_area_key,
      set_category_key = EXCLUDED.set_category_key,
      note = EXCLUDED.note;

-- +goose Down
DELETE FROM import_overrides WHERE match_name IN ('SIS JGC', 'Suzuki');
DELETE FROM category_aliases
 WHERE alias IN ('Sekolah','School','Hospital','Showroom Dealer','Gas Station','University');
DELETE FROM categories
 WHERE key IN ('School','Hospital','Showroom Dealer','Gas Station','University','Other');
DELETE FROM areas WHERE key IN ('Sedayu','JGC','KHI','Metland');
