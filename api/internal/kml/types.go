// Package kml parses a Google Earth KMZ export into facilities and township
// boundaries.
//
// It is a direct port of the browser-side traverseFolder() the app used before
// the refactor, and it deliberately keeps that function's semantics: folder
// context (area and category) is inherited downward, and a placemark whose area
// cannot be resolved is skipped. The one behavioural change is that skipped
// placemarks are now *reported* via Result.Dropped instead of vanishing, because
// every parsing rule here is fitted to a single 47-placemark sample and the next
// export will very likely contain folder names these rules do not recognise.
//
// This package has no database and no HTTP dependencies: it takes bytes plus a
// Rules value and returns a Result. Both the pmctl CLI and the authenticated
// upload endpoint call it, so the two paths cannot drift apart.
package kml

// Coord is a WGS84 longitude/latitude pair. KML stores coordinates as
// "lng,lat,alt", which is the opposite of Leaflet's [lat, lng]; keeping the
// KML/GeoJSON order here means the only place the two conventions meet is the
// API response, where points are emitted as explicit lat/lng fields.
type Coord struct {
	Lng float64
	Lat float64
}

// Ring is a closed sequence of coordinates bounding a polygon.
type Ring []Coord

// Polygon is an outer ring plus zero or more holes.
type Polygon struct {
	Outer Ring
	Inner []Ring
}

// Feature is a facility point: a school, hospital, showroom, and so on.
type Feature struct {
	// ImportKey is the stable natural key used to upsert this row. See ImportKey.
	ImportKey string

	// Name is what the UI displays, after any override is applied.
	Name string
	// RawName is the untouched KML <name>. The import key derives from this, so
	// that renaming a facility through an override does not churn the key and
	// orphan an operator-attached logo.
	RawName string

	AreaKey     string
	CategoryKey string
	// RawCategory is the category inherited from the folder tree before any
	// override. Empty when no ancestor folder named a category. Kept for
	// provenance: it is how you see that "SIS JGC" was filed under Gas Station.
	RawCategory string

	Lat float64
	Lng float64

	// SourceFolder is the "/A/B/C" path the placemark was found at.
	SourceFolder string
	// OverrideApplied records whether an import_overrides row touched this feature.
	OverrideApplied bool
}

// Boundary is a township outline. Stored as a MultiPolygon even though every
// boundary in the current export is a single ring, so a township that later
// splits into disjoint parcels does not fail the import.
type Boundary struct {
	ImportKey   string
	AreaKey     string
	Name        string
	Description string
	// ExternalRef is the "object ID" pulled out of Description, when present.
	ExternalRef string
	Polygons    []Polygon
}

// Dropped records a placemark the parser could not use, and why. Surfaced in
// the import report so a KMZ with unfamiliar folder names fails loudly rather
// than silently importing fewer facilities than it contains.
type Dropped struct {
	Name         string
	SourceFolder string
	Reason       string
}

// Result is everything a single KMZ yielded.
type Result struct {
	Features   []Feature
	Boundaries []Boundary
	Dropped    []Dropped
}

// Override corrects a bad source record. Empty fields mean "leave alone".
type Override struct {
	Name        string
	AreaKey     string
	CategoryKey string
}

// Rules are the data-driven parts of the parse, loaded from the database so
// that correcting a new bad record needs no redeploy.
//
// Area detection is deliberately NOT in here: it is regex logic, and Postgres
// POSIX regex spells the word boundary in \bKHI\b as \y, so keeping it in Go
// avoids a silent semantic divergence between the two implementations.
type Rules struct {
	// CategoryAliases maps a KML folder name to a category key, e.g.
	// "Sekolah" -> "School".
	CategoryAliases map[string]string
	// Overrides maps an exact KML <name> to a correction.
	Overrides map[string]Override
	// FallbackCategory is used when no ancestor folder named a category.
	FallbackCategory string
}
