# Refactor design: static Leaflet site → Next.js + Go + Neon PostGIS

> ## ⚠️ The code is not lost — read this first
>
> **Everything described here was built and is preserved on branch `refactor/fullstack`,
> commit [`edc8ab1`](https://github.com/rinaldo-mist/property-mapper/tree/refactor/fullstack)
> — pushed to GitHub, so it survives this machine.**
>
> | | |
> |---|---|
> | Branch | `refactor/fullstack` (local **and** `origin`, SHAs identical) |
> | Commit | `edc8ab1f7985922289a23ee2f4255f3f96c8f577` — *"refactoring"* |
> | Contents | 66 files · **5,130 lines of Go** · 38 `.go` · 11 `.sql` · migrations · sqlc output · golden fixture · `verify.sh` |
>
> **This document is the reasoning. The branch is the implementation.** Do not re-derive from
> prose what you can `git checkout`. Everything in §5–§9 below exists as working, compiling,
> test-passing code on that branch — the value here is the *why*, and the measurements in §2
> and §7 that the code alone does not explain.

**Status: built, verified, and parked** — `main` was deliberately returned to the vanilla app so
that the multi-group filtering / housing complex work could proceed against it. The two efforts
are independent: if the filtering work ships first, it needs porting into the React frontend
described in §9.

### Resuming

```bash
git checkout refactor/fullstack          # 5,130 lines of Go, exactly as left
docker compose -f deploy/docker-compose.yml up -d postgres   # PostGIS on 5433, see §7.4
cd api && go test ./internal/kml/        # golden test: 43 facilities / 4 boundaries
go run ./cmd/pmctl migrate up
go run ./cmd/pmctl import --file ../data/facility-mapping.kmz   # 43 inserted
go run ./cmd/pmctl import --file ../data/facility-mapping.kmz   # 43 unchanged  <- idempotency
go run ./cmd/server                      # then: bash deploy/verify.sh
```

Pick up at **§11 Phase 3e** — the remaining `verify.sh` assertions (needs `jq`; see §7.4),
testcontainers integration tests, the Dockerfile / Cloud Run deploy, and the entire Next.js
frontend. See §13 for the precise line between done and not-done.

> **Note on the working tree:** `api/`, `web/` and `deploy/` may appear as empty directories on
> `main`. Git does not track empty directories, so switching branches removed their contents but
> left the shells behind. They are inert — safe to delete, and re-created by `git checkout
> refactor/fullstack`.

---

## 1. Why

The app is four files at the repo root: `index.html` (103 lines), `app.js` (320 lines),
`styles.css` (128 lines), and one binary `data/facility-mapping.kmz`. Leaflet 1.9.4 and JSZip
3.10.1 arrive from CDNs at runtime. There is no package.json, no build, no server, no database,
no tests, no CI.

Every visitor downloads the 8 KB KMZ, unzips it in JavaScript, DOM-parses 42 KB of KML, and
walks nested `<Folder>` elements to recover 43 facility points and 4 boundary polygons across
4 townships. Manually added pins live in `localStorage` with base64-encoded logos, so they are
invisible to everyone else and die with the browser profile — a limitation the README already
acknowledges.

The refactor moves the parse to the server and Postgres and rebuilds the UI as a typed React
app. `traverseFolder()` becomes a tested Go package; the `AREA_NAMES` / `CATEGORY_META` /
`CATEGORY_ALIASES` / `FEATURE_FIXES` lookup tables become reference tables; `localStorage` pins
become admin-authored rows visible to everyone; base64 logos become deduplicated `bytea` rows
served with immutable caching. **The map should look essentially identical when it is done.**

---

## 2. Ground truth — measured, not assumed

Extracted from `data/facility-mapping.kmz` by re-implementing `traverseFolder()` independently
and cross-checking against the Go port. **These are the test fixtures.** Any refactor that does
not reproduce them exactly has changed behaviour.

| Fact | Value |
|---|---|
| Points / polygons | **43 / 4** |
| By area | KHI 27 · Sedayu 6 · JGC 6 · Metland 4 |
| By category | School 17 · Showroom Dealer 15 · Hospital 5 · Gas Station 5 · University 1 · **Other 0** |
| Boundary exterior ring points | JGC 229 · Metland 134 · KHI 49 · Sedayu 31 |
| Rings | All closed, no inner rings (no holes) |
| Name collisions | **Zero** — on `area+name` and on name alone |
| Dropped placemarks | **Zero** |
| Geographic centre | ~`[-6.174, 106.963]` (East Jakarta / Bekasi) |

### Two consequences that are easy to get wrong

**`Other` never occurs in the data.** The current app builds chips and the legend from
`[...new Set(allFeatures().map(f => f.category))]`, so it renders **5** chips — while the pin
dialog dropdown offers all **6**. A faithful port must reproduce both numbers. This is why the
API exposes `facilityCount` per category: the UI filters chips on `count > 0` while the dialog
uses the full list.

**Both `FEATURE_FIXES` entries confirmed against the source:**

- `SIS JGC` physically sits in the `JGC / Gas Station` folder. The override corrects only its
  *category* — the area was already right.
- `Suzuki` is a lone `<Placemark>` directly under the root folder
  `Competitor & MR Facility Mapping`, with no area and no category context. It gets both, plus
  a rename to `Suzuki Sedayu`.

### KML structural facts the parser depends on

- Placemark geometry is always a **direct child** of `<Placemark>`. No `MultiGeometry`, no
  `ExtendedData`, no `id` attributes anywhere.
- Boundary polygons live in `<Document>` children of each area `<Folder>`, not in `<Folder>`
  elements — which is why the original parser has a special-case branch for it.
- All 4 boundaries carry `<description>` reading
  `Source: KFMap / Knight Frank Indonesia, object ID N` — free provenance, parsed into
  `external_ref`.
- The document declares both a default namespace and `xmlns:kml` bound to the same URI.

---

## 3. Target architecture

```
browser ──> vercel.app ─┬─> Next.js SSR/static        (Vercel, region sin1)
                        └─> /api/* ──rewrite──> Cloud Run  (asia-southeast1)
                                                    │
                                                    └──> Neon PostGIS (ap-southeast-1)
```

Next.js `rewrites()` proxies `/api/*` to the Cloud Run URL, so **the browser only ever sees the
Vercel origin**. That single fact buys: `SameSite=Lax` is unambiguously correct, CORS never
enters the browser path, and no API hostname is baked into the web build — so the API URL must
**not** be a `NEXT_PUBLIC_` variable.

Everything is co-located in Singapore because the users are in Jakarta. **Set
`"regions": ["sin1"]` in `web/vercel.json`** — Vercel defaults SSR functions to `iad1`
(Virginia), which would make `page.tsx`'s server-side fetch cross the Pacific on every first
paint.

**The stack is serverless at every layer** — Vercel, Cloud Run and Neon all scale to zero and
bill per use. The Dockerfile is a packaging format, not infrastructure. What serverless
*containers* buy over FaaS is a warm process with a real connection pool, which is exactly why
the API can use Neon's **direct** endpoint and keep pgx prepared statements. A per-request FaaS
isolate would force Neon's PgBouncer endpoint plus simple-protocol mode, losing prepared
statements and degrading sqlc's value.

### Cloud Run specifics that are easy to get wrong

1. **Listen on `$PORT`.** Cloud Run injects it; a hardcoded `:8080` fails to start.
2. **SIGTERM grace is 10 s.** Set the shutdown budget to **8 s**, not the 15 you would pick for
   a VPS, or in-flight requests get SIGKILLed mid-drain.
3. **Cap the connection math.** `--max-instances=3` with `pool_max_conns=5` tops out at 15
   connections, inside Neon's free-tier limit. This is what permits the direct endpoint.
4. **CPU is throttled outside request processing** in the default (free-tier-friendly) mode. No
   background goroutine may carry load. CPU is still granted during SIGTERM.
5. **`--allow-unauthenticated`**, since Vercel proxies public traffic. The Cloud Run URL is
   therefore directly reachable — which is fine, and slightly beneficial: the session cookie is
   scoped to the Vercel domain, so a direct hit to Cloud Run cannot carry a session at all.
   Keep `CORS_ALLOWED_ORIGINS` empty so a browser cannot use that URL cross-origin either.

`pmctl` runs in production as a **Cloud Run Job** built from the same image with a different
entrypoint. Migrations run from CI or locally, **never on container start** — a service that
scales out would race itself, which is a data-loss shape.

Cold path: Cloud Run scale-to-zero (~200–400 ms) plus Neon's free-tier idle suspend (~0.5–2 s)
means the first request after a quiet period takes a couple of seconds. Acceptable for an
internal tool; document it before someone reports "the map is slow".

---

## 4. Hosting evaluation (for the record)

| Option | Verdict |
|---|---|
| **Cloud Run** | Chosen. Genuinely free at this scale (2M req/mo, 180k vCPU-s, 360k GiB-s), scales to zero, ~200–400 ms cold start, real container semantics. Cost: GCP project + Artifact Registry + IAM setup. |
| Fly.io | Best Go DX; `sin` region; `auto_stop_machines` scale-to-zero; ~$0–2/mo. Not free since the 2024 change. |
| Railway | Smoothest DX, always-on so no cold start, `asia-southeast1`. **No free tier** — $5/mo Hobby with $5 credit, so ~$0 net but a card is required. |
| Render free tier | Only true $0-no-card option, but sleeps after 15 min idle with a **~50 s** wake. Behind a proxy the user just sees a hung map. |
| Vercel Go functions | Same-origin and one deploy, but serverless: forces Neon's pooled endpoint + simple protocol, Argon2id tuned down, no `pmctl` in prod, and the Go runtime trails the current toolchain. |
| **Netlify** | Investigated and rejected. Go functions are **not** deprecated (contrary to expectation), but Netlify supports only Lambda-style serverless functions — no persistent process, 10–26 s timeout, and explicitly unreliable DB pooling. Strictly worse than Vercel for the same model. |

---

## 5. Database schema

Reference **tables**, not enums, for anything an operator edits or that carries payload.
`CATEGORY_META` carries three attributes per key (label, colour, icon), which an enum cannot
hold, and `ALTER TYPE ... ADD VALUE` is awkward under goose. Only `facility_source` — a closed
internal discriminator with no payload — is a real enum.

Primary keys are the **same TEXT keys the JavaScript used** (`'School'`, `'Showroom Dealer'`,
`'JGC'`). They are already the wire format and already what KML folder names map to, and there
will be ~10 rows forever.

```
00001_extensions      postgis, pgcrypto, citext, pg_trgm + set_updated_at() trigger fn
00002_reference       areas, categories, category_aliases, import_overrides
00003_seed_reference  the four areas / six categories / aliases / two overrides
00004_users_logos     users, logos          -- logos MUST precede facilities (FK)
00005_facilities      facility_source enum, import_batches, facilities, boundaries
```

Extensions are created **in a migration, not an initdb mount**, so a local PostGIS container and
a fresh Neon database go through identical code. One less dev/prod divergence.

```sql
areas            (key PK, label, short_code, sort_order, timestamps)
                 -- short_code: Sedayu|JGC|KHI|MTL — removes the `key==='Metland'?'MTL'` hardcode
categories       (key PK, label, color CHECK ~'^#[0-9a-fA-F]{6}$', icon, sort_order, is_fallback)
                 -- partial unique index enforces exactly one is_fallback ('Other')
category_aliases (alias PK, category_key FK)          -- 'Sekolah' -> 'School'
import_overrides (match_name PK, set_name, set_area_key, set_category_key, note)

users  (id uuid PK, email citext UNIQUE, password_hash, role, is_active,
        token_version int,           -- bump to revoke every outstanding JWT
        last_login_at, timestamps)

logos  (id uuid PK, content_type CHECK IN (png|jpeg|webp|svg+xml),
        byte_size CHECK (>0 AND <= 500000),   -- mirrors the old client-side cap exactly
        sha256 bytea UNIQUE,                  -- content-addressed: dedupe + immutable caching
        width, height, data bytea, created_by, created_at)

facilities (id uuid PK, source facility_source, import_key text,
            name, area_key FK, category_key FK,
            geom geometry(Point,4326) NOT NULL, logo_id FK ON DELETE SET NULL,
            source_folder, raw_name, raw_category, override_applied,   -- provenance
            created_by, updated_by, timestamps, deleted_at,
            CHECK ((source='import') = (import_key IS NOT NULL)))

boundaries (id uuid PK, import_key, area_key FK, name, description, external_ref,
            geom geometry(MultiPolygon,4326) NOT NULL CHECK (ST_IsValid(geom)),
            deleted_at, timestamps)

import_batches (id, source_kind CHECK IN ('cli','upload'), filename, file_sha256, status,
                features_{inserted,updated,unchanged,deleted}, boundaries_{...},
                dropped jsonb,        -- every skipped placemark + why
                error, started_by, started_at, finished_at)

CREATE UNIQUE INDEX facilities_import_key_uk ON facilities (import_key) WHERE source='import';
CREATE INDEX facilities_geom_gix   ON facilities USING GIST (geom);
CREATE INDEX facilities_filter_idx ON facilities (area_key, category_key) WHERE deleted_at IS NULL;
CREATE INDEX facilities_name_trgm  ON facilities USING GIN (name gin_trgm_ops);
```

**`MultiPolygon`, not `Polygon`.** All four boundaries are single rings today, but a township
covering two disjoint parcels would hard-fail a `geometry(Polygon)` column. `ST_Multi()` on
insert costs nothing now versus a data migration later.

**`normalizeArea()` stays in Go, never in the DB.** Postgres POSIX regex has no `\b` — it spells
that `\y` — so a table-driven version would silently disagree with the Go RE2 implementation on
`\bKHI\b`. Aliases and overrides *are* plain string maps and do belong in the database, so a
newly discovered bad record needs a row, not a redeploy.

### One table with a `source` discriminator, not two

The UI has always treated imported facilities and hand-placed pins as one list (`allFeatures()`
concatenates them). Every query path — area, category, bbox, name — is identical for both, and
`ORDER BY name` across a `UNION` is meaningfully worse than across one table. The only real
objection — that an import must never wipe manual pins — is a one-line predicate
(`WHERE source='import'`), enforced structurally by the partial unique index and by the orphan
sweep carrying the same clause.

---

## 6. The `import_key` — the most load-bearing design decision

```
import_key = slug(resolved_area_key) || ':' || slug(raw_kml_name)
             -- e.g. sedayu:suzuki, jgc:sis-jgc
```

Every part of the composition is deliberate:

- **Resolved area, not folder path.** Folder nesting changes between Google Earth exports; the
  township does not.
- **Raw (pre-override) name, not display name.** Editing `import_overrides.set_name` must not
  churn the key. `Suzuki` stays `sedayu:suzuki` while displaying as "Suzuki Sedayu".
- **Category deliberately excluded.** This is the non-obvious one. If category were part of the
  key, removing the `SIS JGC` override would change its key — turning an update into a
  delete-and-recreate, which would **destroy any logo an operator had attached** to that
  facility.
- **Coordinates and ordinals excluded.** Immune to a reordered export and to a surveyor nudging
  a point three metres.

`slug()`: NFKC → lowercase → collapse non-alphanumerics to `-` → trim → truncate at 120 chars,
appending 8 hex chars of `sha256(full)` when truncated.

Collisions are only possible when two facilities share an area *and* a name. **Verified zero in
the real data**, on `area+name` and on name alone. The unique index turns any future collision
into a loud import failure rather than a silently dropped marker.

### The idempotent upsert

```sql
INSERT INTO facilities (...) VALUES (...)
ON CONFLICT (import_key) WHERE source = 'import' DO UPDATE SET
  name = EXCLUDED.name, area_key = ..., category_key = ..., geom = ...,
  source_folder = ..., raw_name = ..., raw_category = ..., override_applied = ...,
  deleted_at = NULL
WHERE  facilities.name IS DISTINCT FROM EXCLUDED.name
   OR  ... OR NOT ST_Equals(facilities.geom, EXCLUDED.geom)
   OR  facilities.deleted_at IS NOT NULL
RETURNING id, (xmax = 0)::boolean AS inserted;
```

Four things this gets right:

1. **`logo_id` is never in the SET list** → an operator-attached logo survives every re-import.
   This is the payoff of the key design above.
2. **`deleted_at = NULL`** → a facility dropped from one export and restored in the next is
   *resurrected with its logo* rather than duplicated. This is also why the unique index
   deliberately omits `AND deleted_at IS NULL` from its predicate.
3. **`(xmax = 0)`** distinguishes insert from update, giving a real operator report.
4. The conditional `WHERE` suppresses no-op updates, keeping `updated_at` honest.

**The consequence of (4) that will bite you:** unchanged rows return **zero rows**, surfacing as
`pgx.ErrNoRows`, which the caller must read as "unchanged" rather than an error. It also means
the orphan sweep **cannot** key off an `import_batch_id` — that would soft-delete every row that
merely did not change. Sweep by the surviving-key array instead:

```sql
UPDATE facilities SET deleted_at = now()
 WHERE source='import' AND deleted_at IS NULL AND import_key <> ALL($1::text[]);
```

Then `unchanged = len(parsed) - inserted - updated`. **Guard the sweep against an empty array** —
a KMZ that parses to zero facilities means a broken export, not an empty one, so the import must
abort rather than soft-delete the whole dataset.

A re-import reporting **`43 unchanged`** is the operator-legible signal that nothing moved.

---

## 7. Findings that cost real effort

### 7.1 sqlc handles PostGIS — verified, no fallback needed

sqlc has no notion of the `geometry` type, which was the single largest technical risk in the
plan. **Mitigation: never `SELECT geom` raw.** Always project it:

```sql
ST_X(f.geom)::float8 AS lng,
ST_Y(f.geom)::float8 AS lat,
ST_AsGeoJSON(b.geom)::text AS geometry
```

sqlc then only ever sees `float8` and `text`. Since GeoJSON is the wire format anyway, this is a
natural fit rather than a workaround. Add a `sqlc.yaml` override mapping `geometry → string` as
a safety net for the schema scan itself.

**Spike result (sqlc v1.31.1, three representative queries):** generated cleanly, producing
`Lng float64` / `Lat float64` from the projections, `Inserted bool` from
`(xmax = 0)::boolean`, and `*string` / `[]string` for optional filters via `sqlc.narg` and
`cardinality(...) = 0`. The pgx + hand-written-scan fallback (≈1 day of work) is **not needed**.

### 7.2 The JGC boundary is genuinely invalid at source

`ST_MakeValid` in the boundary upsert is **load-bearing, not precautionary**.

```
raw_valid     f
reason        Self-intersection[106.951776407767 -6.18267805825243]
raw_pts       229
fixed_pts     232
fixed_type    MULTIPOLYGON
raw area      3 978 744 m²
fixed area    3 978 742 m²      (0.00005% change)
hausdorff     0.000000000000°   (geometrically identical)
```

Without the repair, `CHECK (ST_IsValid(geom))` rejects the JGC row and **the entire import
fails**. The repair nodes the self-intersection, adding 3 points; the outline does not move.

**Therefore expect two different, both-correct ring counts:** PostGIS stores JGC **232**, while
the parser's own golden test asserts the pre-repair **229**. Metland 134 / KHI 49 / Sedayu 31 are
unaffected. Anyone "fixing" this discrepancy later will break the import.

A repair yielding a `GeometryCollection` still fails loudly, since `ST_Multi` cannot coerce one —
which is the intended behaviour for genuinely broken input.

### 7.3 Behaviour parity proven mechanically

An independent re-implementation of the original `app.js` `traverseFolder()` logic, written from
the JavaScript rather than from the Go port, was compared against the populated database:

```
legacy(app.js logic): 43   database: 43
DIFF: <empty> — every name, area, category and coordinate matches to 7 decimal places
```

This is the definitive proof the refactor preserved behaviour, and it is cheap to re-run.

### 7.4 Local environment

- **Local Postgres must bind 5433.** This machine already runs a `postgresql-x64-18` Windows
  service on 5432; the compose stack silently shadowing it produces a confusing
  `password authentication failed` error. Do not stop the user's service.
- `make` is not installed → use `Taskfile.yml` (go-task).
- `jq` is not installed by default → `winget install jqlang.jq` (needed by `verify.sh`).
- Toolchain at time of work: Go 1.25.4 (go.mod resolved to 1.26.0 via goose v3.28), Node 24.11.1,
  Docker 29.1.3.
- Docker Compose **interpolates every service regardless of active profile**, so a `${VAR:?err}`
  guard on a profiled service breaks unrelated commands like `db-up`. Use `${VAR:-}` and let the
  application's fail-fast config do the validating.

---

## 8. Go backend

| Concern | Choice | Rationale |
|---|---|---|
| Router | **chi v5** | `r.Group` / `r.With(auth)` makes the public-vs-admin boundary auditable at a glance. Forgetting to wrap one mutating route is a real security bug. |
| DB | **pgx/v5 + pgxpool + sqlc** | PostGIS forces hand-written SQL anyway. GORM rejected — you would write `db.Raw()` everywhere and lose its only benefit. sqlc catches column drift at build time. |
| Migrations | **goose v3** | `StatementBegin/End` is required for the trigger function body; golang-migrate's `;`-splitting breaks on it. Embeds via `embed.FS`. |
| Config | hand-written over `os.Getenv` | ~12 vars. Collect **all** problems and fail at boot with the full list. |
| Logging | **`log/slog`** | JSON in prod (Cloud Logging parses `severity` natively), text in dev. No dependency. |
| Password | **`alexedwards/argon2id`** | Full params `m=64MiB, t=3, p=2`, affordable because this is a container not a FaaS isolate. Gate concurrent verifies with a semaphore of 4 (256 MiB transient). |
| JWT | **`golang-jwt/jwt/v5`** | HS256; claims `sub`, `ver` (→ `users.token_version`), `exp` 12h. Pin the signing method to reject `alg: none`. |
| Rate limit | `golang.org/x/time/rate` | In-memory, keyed IP+email, on `/auth/login` only. |
| Testing | testify + testcontainers-go | The DB layer *is* PostGIS behaviour; mocking it tests nothing. |

**`internal/httpapi.Router(deps) http.Handler` is the only place routes are declared** — mounted
by `cmd/server` and directly by `httptest`. Beyond testability, this is what makes the host
swappable: moving off Cloud Run touches the Dockerfile and CI, not the application.

### KML parsing: `encoding/xml` structs + a manual context walk

Two genuinely separate phases, so two pieces of code.

**Phase 1 — shape.** A recursive `container` struct serving both `<Folder>` and `<Document>`.
Two properties of `encoding/xml` do real work for free:

- A field tagged `xml:"Folder"` with **no namespace URI** matches that element in *any*
  namespace — exactly reproducing the JS `el.localName === tag` check. **Do not put the KML
  namespace URI in the tags.**
- Struct tags match **direct children only**, which is precisely `directChildren()` semantics.

**Phase 2 — semantics.** A plain recursive `walk(c container, ctx walkContext)`, a line-for-line
translation of `traverseFolder()`, testable with zero XML. Doing this inside an `xml.Decoder`
token loop would mean hand-managing a context stack and would lose that testability.

Two deliberate divergences, both no-ops on current data, both strict improvements:

- JS used `getElementsByTagNameNS('*','Point')` — a *descendant* search. Go matches direct
  children. Identical today, but a future `<MultiGeometry>` would silently vanish, so handle it
  explicitly (~10 lines).
- JS took `coordinates[0]`, which merely *happened* to be the outer ring. Go addresses
  `outerBoundaryIs>LinearRing>coordinates` explicitly, staying correct if holes ever appear.

**One shared method** — `RunImport(ctx, r io.ReaderAt, size int64, meta Meta) (*Report, error)`.
`pmctl` passes an `*os.File`; the upload handler passes a `bytes.Reader`. Both load `Rules`
(aliases + overrides) from the database, so the two paths cannot drift.

Because every parsing rule is fitted to a **single 47-placemark sample**, the importer records
every unresolvable placemark into `import_batches.dropped` with a reason, and both the CLI and
the import UI surface it prominently. The original parser silently `return`ed instead.

### Endpoints

```
Public
  GET /api/v1/health · /ready          -- /ready is the Cloud Run startup probe
  GET /api/v1/catalog                  {areas:[{key,label,shortCode,sortOrder,facilityCount}], categories:[...]}
  GET /api/v1/facilities               ?area=&category=&category=&q=&bbox=&limit=&offset=
  GET /api/v1/boundaries               ?area=
  GET /api/v1/map                      the single bootstrap call
  GET /api/v1/logos/{id}               image bytes, ETag + immutable cache
Auth
  POST /api/v1/auth/login              {email,password} -> 204 + Set-Cookie | 401 generic
  POST /api/v1/auth/logout · GET /api/v1/auth/me
Admin  (RequireAdmin + X-Requested-With)
  POST /api/v1/pins · PATCH /api/v1/pins/{id} · DELETE /api/v1/pins/{id}
  POST /api/v1/logos                   multipart file=<image>
  POST /api/v1/import                  multipart file=<kmz>
  GET  /api/v1/import/batches
```

Pin mutations carry `AND source='manual'` → a 404 if an admin targets an imported facility,
preserving the old "only manual pins are editable" rule in one predicate.

**Wire shapes:** points as `lat`/`lng` scalars (feeds `L.marker([lat,lng])` directly);
boundaries as GeoJSON so the frontend uses `L.geoJSON` with **no coordinate flipping anywhere** —
a real simplification over `parseCoordinates`'s manual swap. Emit `source`, let TypeScript derive
`isManual`; do not ship a redundant `manual` boolean.

**Logo URLs are relative** (`/api/v1/logos/<uuid>`) so they resolve against the page origin and
work through the rewrite proxy without the API knowing its own public hostname.

### Security

**CSRF:** the proxy makes everything same-origin, so `SameSite=Lax` is correct. But
`POST /api/v1/import` is `multipart/form-data` — a **CORS-simple content type**, therefore
reachable by a plain cross-site `<form>` POST that Lax alone does not stop. **Require
`X-Requested-With: pm-admin` on every mutating route**; a custom header forces a preflight,
which a simple form cannot send. Mount this middleware once on the whole `/api/v1` group where
it is a no-op on safe methods — that makes it structurally impossible to ship an unprotected
mutating endpoint. Also reject `Sec-Fetch-Site: cross-site` on state-changing routes.

**Login:** identical response for unknown email, wrong password and disabled account, and a
dummy Argon2id verify on the unknown-email path so timing does not leak which addresses exist.

**Fail-fast config:** refuse to boot if `AUTH_JWT_SECRET` < 32 bytes, or if
`APP_ENV=production` with `AUTH_COOKIE_SECURE=false`.

**SVG logos:** an uploaded SVG can carry `<script>`. It never executes inside an `<img>`, so the
only exposure is direct navigation to the logo URL — closed with
`Content-Security-Policy: default-src 'none'; sandbox`, `X-Content-Type-Options: nosniff` and
`Content-Disposition: inline`. Content-type is validated against **both** the declared type and
`http.DetectContentType`; note SVG sniffs as `text/xml`, needing an explicit case.

### Logo storage: Postgres `bytea`

Chosen over a filesystem volume or S3/MinIO on an **operational**, not performance, argument:
for a small internal tool, "restore the database" must mean "the app is fully back", and one
`pg_dump` achieving that is worth more than the TOAST overhead of ~20 images at ≤500 KB.
Content-addressed via `sha256 UNIQUE` gives free dedupe and makes
`Cache-Control: immutable` provably safe — the bytes behind an id can never change.

---

## 9. Next.js frontend

Next 15 App Router, React 19, TypeScript strict, Tailwind v4, Leaflet 1.9.4 from npm.

**Map library → react-leaflet v5.** MapLibre rejected outright: a WebGL/vector model means
redesigning the `divIcon` HTML markers, the rotated-teardrop CSS pin and all popup DOM — it
cannot satisfy "look essentially the same", and needs a keyed tile source where OSM raster is
free. Raw Leaflet in a `useEffect` is the most mechanical port but carries forward the
`clearLayers()`-and-rebuild pattern where **three of the four known bugs live**. react-leaflet
renders the *same DOM* Leaflet does, so the CSS — which determines the look — is unchanged and
there is no visual risk.

Leaflet touches `window` at module scope, so the map subtree is `dynamic(..., { ssr: false })`,
reusing the existing `.loading` card as the fallback. Self-host fonts via `next/font/google` and
import `leaflet/dist/leaflet.css` from npm — killing two CDN dependencies and the layout shift.
**JSZip leaves the frontend entirely** (Go's `archive/zip` owns it now).

`app/page.tsx` is a Server Component fetching `/api/v1/map` and passing the payload as props,
giving a fully-populated first paint — strictly better than today's blank-until-KMZ-parsed.
TanStack Query hydrates from it via `initialData`; the dependency earns its place because
`router.refresh()` can churn the client boundary owning the Leaflet instance, whereas
`invalidateQueries(['map'])` leaves the map untouched.

**Filtering stays 100% client-side** over ~43 rows via `useMemo`. The current UX is instant on
every keystroke; a round trip makes it worse. The API accepts `area`/`category`/`q`/`bbox` for
curl and tests, but the UI does not use them — revisit past ~2,000 points, at which point `bbox`
already exists.

**Filter state → URL search params**, so filters are shareable. Two traps: debounce the URL sync
(~200 ms) while keeping the input value local, or typing feels laggy; and **encode "all" vs
"none" explicitly** (absent `cat` = all, `?cat=` = none) — conflating them reintroduces the
category bug in a new costume.

**Keep the native `<dialog>`.** It already provides `::backdrop`, focus trap and Esc-to-close,
all of which the current app relies on. Do not reach for Radix/shadcn.

### Tailwind v4 theme — three things that must NOT become utilities

v4 is CSS-first and emits its theme as custom properties, so `styles.css`'s `:root` maps nearly
1:1 into `@theme` (`--color-ink: #162019`, `--color-paper: #f6f7f3`, `--color-green: #174c36`…),
plus `--breakpoint-md: 760px` to **match the existing breakpoint exactly rather than Tailwind's
768**.

1. **Category colours are data**, chosen per row from the API. **Never build `bg-${category}`** —
   Tailwind's static extraction cannot see it and will purge it. Use
   `style={{background: category.color}}`, exactly as today. This is the single most common
   Tailwind-port mistake.
2. **Leaflet's own DOM** (`.leaflet-popup-content-wrapper`, `.leaflet-control-*`) cannot take
   utility classes — port `styles.css:78-83` verbatim into a plain stylesheet.
3. **`.marker-pin`** — the `rotate(-45deg)`/`rotate(45deg)` teardrop trick reads far better as
   plain CSS in `@layer components`, and `divIcon` injects raw HTML anyway.

**Auth in the UI:** hide (not disable) admin affordances when anonymous. **No `middleware.ts`
route gating** — that duplicates JWT verification in TypeScript. The API is the single
authority; the UI only hides affordances.

**Two copy fixes**, because the current strings become false: "Membaca data KMZ…" →
"Memuat data fasilitas…", and "Pin manual tersimpan di browser ini" → "…tersimpan di server".

---

## 10. Known bugs in the original `app.js`

| Bug | Notes |
|---|---|
| `app.js:180` — result-list click uses `state.features.find`, so clicking a **manual** pin does nothing | `allFeatures()` at `app.js:121` is the intended lookup |
| `app.js:201-202` — `renderControls()` re-adds every category on every render, silently re-enabling chips the user turned off | Vanishes under a faceted model where selection is not rebuilt from data |
| `app.js:156-158` — `popupopen` stacks a listener per reopen | Worse than a leak: the *N*th reopen calls `showModal()` on an already-open dialog and throws `InvalidStateError` |
| `app.js:83` — feature id embeds a folder-local index | **Latent, not active** — zero collisions verified in this data. Also puts spaces and commas into `data-id` / `CSS.escape` |

**A fifth issue not on that list, which the React port would introduce:** `fitVisible()` runs on
load, on "Lihat semua" and on developer toggle — **but not on search or category change**. The
natural port is a `useEffect` on `[visibleFeatures]`, which would **yank the map on every
keystroke**. Preserve the trigger set exactly, via imperative calls behind a ref.

---

## 11. Execution order

Parser and importer land **before any HTTP or React**, so the behaviour-critical piece is proven
while the feedback loop is still a millisecond `go test`.

```
Phase 0  scaffold: branch, git mv originals -> legacy/, api/ web/ deploy/, Taskfile, .env.example
Phase 1  data layer (riskiest first)
         2. docker compose postgres (port 5433) + Neon project in ap-southeast-1
         3. goose 00001..00005 — verify up / down×5 / up all clean
         4. sqlc spike — THE GO/NO-GO GATE, before 30 queries exist
Phase 2  parser + importer, no HTTP
         5. internal/kml — golden test at 43/4 with exact histograms
         6. internal/importing + pmctl import — 43 inserted, then 43 unchanged
         7. pmctl admin create
Phase 3  API: router+server -> public reads -> auth -> pins/logos/import
Phase 4  Dockerfile -> Artifact Registry -> Cloud Run + pmctl Job
         (deploy BEFORE the frontend, so the proxy target is real from the first line of React)
Phase 5  Next.js: scaffold+theme -> read-only UI -> admin UI
Phase 6  CI, README, delete legacy/ after screenshot sign-off
```

`legacy/` exists to serve the pre-refactor app side by side (`npx serve legacy`) as a visual
baseline — the cheapest substitute for visual-regression tooling.

---

## 12. Verification

**A. Row-count parity (primary gate)**

```sql
SELECT count(*) FROM facilities WHERE source='import' AND deleted_at IS NULL;  -- 43
SELECT count(*) FROM boundaries WHERE deleted_at IS NULL;                      -- 4
SELECT area_key, count(*) FROM facilities GROUP BY 1 ORDER BY 1;
    -- JGC 6 | KHI 27 | Metland 4 | Sedayu 6
SELECT category_key, count(*) FROM facilities GROUP BY 1 ORDER BY 1;
    -- Gas Station 5 | Hospital 5 | School 17 | Showroom Dealer 15 | University 1
SELECT raw_name, name, area_key, category_key FROM facilities
 WHERE raw_name IN ('SIS JGC','Suzuki');
    -- SIS JGC | SIS JGC       | JGC    | School
    -- Suzuki  | Suzuki Sedayu | Sedayu | Showroom Dealer
SELECT area_key, ST_NPoints(geom) FROM boundaries ORDER BY 1;
    -- JGC 232 (post-MakeValid) | KHI 49 | Metland 134 | Sedayu 31
```

**B. Coordinate parity — the definitive proof, ~10 minutes.** In the browser console on the
legacy app:

```js
copy(JSON.stringify(state.features.map(f =>
  [f.name, f.area, f.category, +f.latlng[0].toFixed(7), +f.latlng[1].toFixed(7)]).sort()))
```

Reshape `/api/v1/map` into the same form, round both to 7 decimals, `diff`. **Expect empty.**

**C. Endpoint smoke** (`deploy/verify.sh`, runnable against localhost *and* the live Cloud Run
URL): `/catalog` → 4 areas and **6** categories but only **5** with `facilityCount > 0`;
`?area=KHI` → 27; `?category=School` → 17; `?category=School&category=Hospital` → 22;
`?q=shell` → 1; unauthenticated mutations → 401; missing `X-Requested-With` → 403.

**D. Idempotency + non-clobber — the crown jewel.** Import → create a manual pin and attach a
logo → import again → `facilities` = 44, the manual pin and its `logo_id` untouched, batch
reports `0/0/43/0`. This proves the one-table/`source` design does not clobber manual data.

**E. Visual parity.** Legacy on :5000 vs the new app on :3000, screenshotted at 1440×900 and
390×844 in six states: initial load, KHI selected, search `penabur`, popup open, mobile drawer,
pin dialog. Accept **only** the two intentional copy fixes.

**F. Manual checklist** — what tests will not catch: search does **not** refit the map, developer
toggle **does** · toggle a category off then save a pin, it stays off · click a **manual** pin in
the results list, the map flies · open a manual popup 5× and click Edit, exactly one dialog and
no console error · legend shows 5, dropdown shows 6 · developer codes read `Sedayu / JGC / KHI /
MTL` · `fitBounds` padding `[55,55]` maxZoom 15, `flyTo` zoom 17 duration 0.8 s, popup after
700 ms.

---

## 13. Status — where the code is

Every path below is on `refactor/fullstack` @ `edc8ab1`. Read this table alongside the design
sections it points at; the code is the source of truth, this document explains the decisions.

| Design section | Implemented in | Notes |
|---|---|---|
| §5 schema | `api/db/migrations/0000{1..5}_*.sql` | + `api/db/migrations.go` embeds them via `embed.FS` |
| §5 seed data | `00003_seed_reference.sql` | areas / categories / aliases / both overrides, `ON CONFLICT DO UPDATE` so it is re-runnable |
| §6 import key | `api/internal/kml/slug.go` | `Slug()` + `ImportKey()`, with the "excludes category" reasoning in the doc comment |
| §6 upsert + sweep | `api/internal/store/queries/facilities.sql` | `UpsertImportedFacility`, `SweepOrphanedFacilities` |
| §7.1 sqlc config | `api/sqlc.yaml` | the `geometry → string` override safety net |
| §7.2 `ST_MakeValid` | `api/internal/store/queries/boundaries.sql` | the JGC measurements are recorded in the SQL comment, where someone about to "simplify" it will see them |
| §8 KML parser | `api/internal/kml/{types,parse,rules,kmz}.go` | two-phase: `encoding/xml` structs, then `walk()` |
| §8 parser tests | `api/internal/kml/parse_test.go` | golden test at 43/4 + both overrides + ring fingerprints; `-update` regenerates |
| §8 importer | `api/internal/importing/service.go` | `RunImport(io.ReaderAt, ...)` — the one method CLI and HTTP share |
| §8 endpoints | `api/internal/httpapi/{server,public,auth,pins,logos,importer}.go` | `server.go` holds the whole route table |
| §8 CSRF / errors | `api/internal/httpx/{middleware,errors,respond}.go` | `RequireRequestedWith` is mounted once on `/api/v1` |
| §8 auth | `api/internal/auth/{password,token}.go` | Argon2id params + the login semaphore; JWT with `token_version` |
| §3 Cloud Run | `api/cmd/server/main.go` | `$PORT`, 8 s shutdown grace, `-health` self-probe for distroless |
| operator CLI | `api/cmd/pmctl/{main,migrate,import,admin}.go` | `stubs.go` still holds a `runLogos` placeholder |
| §12 verification | `deploy/verify.sh`, `testdata/facilities.golden.json` | the fixture is shared with the intended Vitest suite |
| local stack | `deploy/docker-compose.yml`, `Taskfile.yml` | PostGIS on **5433**, see §7.4 |

**Built and verified:**

- Repo scaffold, `Taskfile.yml`, compose (PostGIS on **5433**), `.env.example`
- `internal/kml` — full parser, **all golden tests passing** at 43/4 with exact histograms,
  boundary ring fingerprints, and both overrides asserted
- `testdata/facilities.golden.json` — 43 records, shared fixture for Go and (intended) Vitest
- Migrations 00001–00005, verified **up → down×5 → up** clean
- sqlc spike **passed**, then the full query layer (catalog, facilities, boundaries, logos,
  users, import batches) generated and compiling
- `internal/importing` + `pmctl import` — **43 inserted, then 43 unchanged** on re-run
- `pmctl migrate` / `pmctl admin create` — admin row with a real `$argon2id$` hash
- `internal/auth` (Argon2id + JWT), `internal/httpx` (errors, respond, middleware),
  `internal/httpapi` (router, public reads, auth, pins, logos, import), `cmd/server` with
  `$PORT` + 8 s graceful shutdown — **all building and serving**
- `deploy/verify.sh` — 11 checks passing, **the entire auth/CSRF boundary green**

**Not done:** the remaining `verify.sh` assertions (blocked only on `jq` not being installed),
testcontainers integration tests, Dockerfile / Cloud Run deploy, and the whole Next.js frontend.

---

## 14. Decision log — what was asked, weighed, and agreed

The full record of the planning discussion. Code shows *what* was built; this shows *why*, and
what was rejected. Read this before overturning anything below — several options that look
attractive in isolation were considered and ruled out for reasons that are not obvious.

### D1 · Who may create, edit and delete pins → **admin login**

Moving `localStorage` pins into Postgres makes them shared and globally visible, which forces the
question the old app never had to answer.

| Option | Verdict |
|---|---|
| **Admin login** — public read-only map, email + password, Argon2id, HttpOnly JWT cookie gating writes | **Chosen.** The only option safe to put on a public URL. Costs a `users` table and a login screen. |
| Fully public writes | Closest to today's behaviour and simplest to build, but an open write endpoint anyone can vandalise or fill with junk images. |
| Shared admin API key | Less code than real auth, but the secret cannot be rotated per person or revoked individually. |

### D2 · How the KMZ feeds Postgres → **import CLI *and* upload endpoint**

| Option | Verdict |
|---|---|
| Go import CLI only, DB is source of truth | Sound, but requires shell access for every data refresh. |
| **CLI + authenticated `POST /api/v1/import`** | **Chosen.** The endpoint is what makes refreshes possible with no shell — and it became essential later: under D5's original serverless answer, the CLI could not run in production at all. |
| Parse the KMZ at request time | Rejected: no queryability, no history, no spatial indexing. |

Consequence: both paths must share one implementation, hence `RunImport(io.ReaderAt, …)` in
`internal/importing`, with parsing rules loaded from the database so the two cannot drift.

### D3 · Geospatial storage → **PostGIS**

Chosen over plain `double precision` lat/lng plus `jsonb` rings. Real spatial types, GiST
indexes, `ST_AsGeoJSON` straight out of the database, and genuine bbox queries. The accepted
cost is that the database image must carry the extension — which Neon does. This decision later
made §7.2 (the invalid JGC boundary) *detectable*; the plain-columns option would have stored the
self-intersecting ring silently.

### D4 · UI fidelity → **port the existing look into Tailwind v4**

| Option | Verdict |
|---|---|
| **Port `styles.css` into Tailwind v4 `@theme` tokens** | **Chosen.** Visually near-identical, conventional and maintainable underneath. |
| Keep `styles.css` verbatim as CSS Modules | Lowest visual risk, but keeps the hand-rolled CSS the rest of the stack is moving away from. |
| Redesign with shadcn/ui | Rejected: the app would visibly stop looking like itself — beyond a refactor. |

Explicitly agreed: **Indonesian copy is retained throughout**, with exactly two strings changed
because the refactor makes them false ("Membaca data KMZ…", "…tersimpan di browser ini").

### D5 · Hosting → **Cloud Run** (after three reversals — the reasoning matters)

This changed direction three times. The full path is recorded because each turn eliminated a
real option, and re-litigating it would cost the same effort again.

1. **Started at "single host, docker-compose."** Asked whether Vercel could host everything.
2. **→ All on Vercel (Go serverless functions).** Chosen for one deploy, one domain, native
   same-origin cookies. **Accepted costs at the time:** Neon's pooled endpoint with pgx in
   simple-protocol mode (losing prepared statements), Argon2id tuned down to fit function memory,
   no `pmctl` in production, and a Go runtime trailing the current toolchain.
3. **→ Reversed: "Go lives on Railway or another free provider."** Recommendations given, with
   the correction that **Railway has no free tier** — it ended in 2023; Hobby is $5/mo with $5
   credit, so ≈$0 net but a card is required.
4. **→ "Can Netlify do it?"** Researched rather than assumed. Finding: **Netlify's Go support is
   not deprecated**, contrary to expectation — but it offers *only* Lambda-style functions: no
   persistent process, 10–26 s timeout, and explicitly unreliable DB pooling. It would have
   reinstated every serverless compromise from step 2, in a weaker form than Vercel. Rejected.
5. **→ Google Cloud Run.** Chosen. See §4 for the full comparison.
6. **→ "Can we make this serverless?"** Clarified that it already is: Cloud Run scales to zero,
   bills per request, and has no managed server — the Dockerfile is packaging, not
   infrastructure. Vercel and Neon are likewise serverless. Confirmed: keep the plan.

**The decisive property** is that serverless *containers* keep a warm process with a real
connection pool. That is what lets the API use Neon's **direct** endpoint and keep pgx prepared
statements — and it is what reverted every compromise listed in step 2. A per-request FaaS
isolate would force them all back.

### D6 · Existing `localStorage` pins → **no migration path**

A one-time "import pins from this browser" button was offered (≈60 lines: read the key, POST each
pin, convert each base64 logo into a real upload, clear the key). Declined — no pins worth
saving, and it would have left dead code behind.

### D7 · Test scope → **core + integration, no Playwright**

Agreed: KML golden test pinned to the 43/4 fixture, importer idempotency against real PostGIS via
testcontainers, an auth matrix over every route, query filters, and Vitest over the pure filter
function using the same shared fixture. A full Playwright E2E tier was offered and declined as
disproportionate for a ~50-row internal tool.

### D8 · Decisions taken without asking

Judgement calls made during design, recorded so they can be revisited deliberately.

| Decision | Reasoning |
|---|---|
| **One `facilities` table with a `source` discriminator**, not two tables | The UI already merges them; every query path is identical; `ORDER BY name` across a `UNION` is worse. The "imports must not wipe manual pins" objection is a one-line predicate, not an architecture. |
| **`geometry(MultiPolygon)`**, not `Polygon` | All four boundaries are single rings today, but a township with two disjoint parcels would hard-fail the column. `ST_Multi()` now costs nothing; a data migration later would not. |
| **Logos as Postgres `bytea`**, not filesystem or S3 | Operational, not performance: `pg_dump` must mean "the app is fully restorable". Weighed against a volume (a second thing to back up, orphan reaping, breaks with >1 replica) and S3/MinIO (a container, credentials and an SDK to store ~10 MB). |
| **Keep `Other` / "Lainnya"** as the fallback category | Never occurs in the data, but it is what an unrecognised folder name falls back to, and the old pin dialog offered it. Removing it would change dialog behaviour. |
| **Keep SVG in the logo allowlist** | The old form accepted it. The `<script>` risk is closed with `CSP: default-src 'none'; sandbox` + `nosniff` on that response, rather than by dropping a supported format. |
| **`X-Requested-With` as the CSRF defence** | `SameSite=Lax` covers the JSON endpoints, but `POST /import` is `multipart/form-data` — a CORS-*simple* type reachable by a cross-site `<form>`. A custom header forces a preflight. Mounted once on `/api/v1` so no mutating route can miss it. |
| **react-leaflet**, not MapLibre or raw Leaflet | MapLibre cannot satisfy "look essentially the same" — different rendering model, and it needs a keyed tile source. Raw Leaflet in an effect would carry forward the teardown-and-rebuild pattern where three of the four known bugs live. |
| **Filtering stays client-side** | ~43 rows. The current UX is instant per keystroke; a round trip makes it worse. The API still accepts `area`/`category`/`q`/`bbox` for curl and tests. Revisit past ~2,000 points. |
| **chi** over stdlib `ServeMux` | `r.Group` / `r.With(auth)` makes the public-vs-admin boundary auditable at a glance. Still a plain `http.Handler`, so reverting is about an hour. |
| **sqlc** over GORM or sqlx | PostGIS forces hand-written SQL regardless; GORM would mean `db.Raw()` everywhere, losing its only benefit. sqlc catches column drift at build time. |
| **goose** over golang-migrate | `StatementBegin/End` is required for the `set_updated_at()` function body; golang-migrate's `;`-splitting breaks on it. |
| **TanStack Query** despite the extra dependency | After a mutation or import you need a refetch, and `router.refresh()` can churn the client boundary owning the Leaflet instance. `invalidateQueries` leaves the map untouched. |
| **URL search params** for filter state | Filters become shareable and back works, with no dependency. Rejected zustand: four pieces of state, one of them ephemeral. |

### D9 · Deferred, never resolved

- **A second KMZ export to test against.** Still the highest-value missing input — see §15.1.
- **Who runs backups**, and on what schedule.
- **Optimistic locking on pins.** Flagged, deliberately not built.

## 15. Open risks

1. **⚠️ Only one KMZ sample exists.** Every rule — the four `normalizeArea` regexes, the aliases,
   both overrides — is fitted to a single 47-placemark export. The next one will plausibly carry
   folder names that do not match (`Rumah Sakit`, `SPBU`). Mitigated by the `dropped` reporting,
   but **testing against a second export is the highest-value thing available.**
2. **Cold start is two-layered** — Cloud Run scale-to-zero plus Neon idle suspend. A couple of
   seconds on the first hit after idle. `--min-instances=1` fixes half of it but leaves the free
   tier.
3. **Backups are unowned.** With logos in `bytea`, `pg_dump` is the entire backup. Neon branching
   and PITR cover the free tier's retention window; document the command and the limit.
4. **Free-tier terms change.** Verify Cloud Run's always-free allowance and Neon's free plan at
   signup rather than trusting this document.
5. **No optimistic locking on pins** — two admins editing the same pin is last-write-wins. Fine
   at this scale; add `version` + `If-Match` only if it ever matters.
