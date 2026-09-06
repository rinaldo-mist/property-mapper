-- +goose Up
-- Reference tables, not enums.
--
-- The line is drawn at payload and editability: anything an operator edits, or
-- that carries attributes, is a table. CATEGORY_META carried three attributes per
-- key (label, colour, icon), which an enum cannot hold, and ALTER TYPE ... ADD
-- VALUE is awkward under goose. Only facility_source — a closed internal
-- discriminator with no payload — is a real enum (migration 00005).
--
-- Primary keys are the same TEXT keys the old JavaScript used ('School',
-- 'Showroom Dealer', 'JGC'). They are already the wire format and already what
-- KML folder names map to, and there will be roughly ten rows forever.

CREATE TABLE areas (
  key        text PRIMARY KEY CHECK (key ~ '^[A-Za-z][A-Za-z0-9_-]{0,31}$'),
  label      text NOT NULL CHECK (length(btrim(label)) > 0),
  -- short_code removes the `key === 'Metland' ? 'MTL' : key` hardcode the
  -- sidebar used to carry.
  short_code text NOT NULL CHECK (length(btrim(short_code)) > 0),
  sort_order int  NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE categories (
  key         text PRIMARY KEY CHECK (key ~ '^[A-Za-z][A-Za-z0-9 _-]{0,31}$'),
  label       text NOT NULL CHECK (length(btrim(label)) > 0),
  color       text NOT NULL CHECK (color ~ '^#[0-9a-fA-F]{6}$'),
  icon        text NOT NULL CHECK (length(icon) BETWEEN 1 AND 8),
  sort_order  int  NOT NULL DEFAULT 0,
  -- Exactly one category is the fallback used when no folder named a category.
  is_fallback boolean NOT NULL DEFAULT false,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX categories_one_fallback ON categories (is_fallback) WHERE is_fallback;

-- CATEGORY_ALIASES: folds Indonesian folder names into canonical category keys.
CREATE TABLE category_aliases (
  alias        text PRIMARY KEY,
  category_key text NOT NULL REFERENCES categories(key) ON UPDATE CASCADE ON DELETE CASCADE
);

-- FEATURE_FIXES as data, so correcting a newly discovered bad record in the next
-- KMZ export needs a row, not a redeploy. NULL means "leave this field alone".
CREATE TABLE import_overrides (
  match_name       text PRIMARY KEY,
  set_name         text,
  set_area_key     text REFERENCES areas(key)      ON UPDATE CASCADE,
  set_category_key text REFERENCES categories(key) ON UPDATE CASCADE,
  note             text,
  CONSTRAINT import_overrides_not_empty
    CHECK (set_name IS NOT NULL OR set_area_key IS NOT NULL OR set_category_key IS NOT NULL)
);

CREATE TRIGGER areas_set_updated_at
  BEFORE UPDATE ON areas FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER categories_set_updated_at
  BEFORE UPDATE ON categories FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS import_overrides;
DROP TABLE IF EXISTS category_aliases;
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS areas;
