package httpapi

import (
	"encoding/json"
	"time"
)

// Wire types.
//
// Two deliberate choices shape these:
//
//   - Points are lat/lng scalars, matching what L.marker([lat, lng]) wants.
//     Boundaries are GeoJSON, which L.geoJSON consumes directly. Between them,
//     no coordinate order is ever swapped in the frontend — the old client had
//     to flip every KML lng,lat pair by hand.
//   - Facilities carry `source`, not a redundant `manual` boolean. TypeScript
//     derives isManual from it, so there is one field to keep consistent.

type Area struct {
	Key           string `json:"key"`
	Label         string `json:"label"`
	ShortCode     string `json:"shortCode"`
	SortOrder     int    `json:"sortOrder"`
	FacilityCount int    `json:"facilityCount"`
}

type Category struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Color     string `json:"color"`
	Icon      string `json:"icon"`
	SortOrder int    `json:"sortOrder"`
	// IsFallback marks the category used when a KMZ folder names none.
	IsFallback bool `json:"isFallback"`
	// FacilityCount lets the UI render chips only for categories that actually
	// occur — five today — while the pin dialog still offers all six.
	FacilityCount int `json:"facilityCount"`
}

type Facility struct {
	ID          string  `json:"id"`
	Source      string  `json:"source"` // "import" | "manual"
	Name        string  `json:"name"`
	AreaKey     string  `json:"areaKey"`
	CategoryKey string  `json:"categoryKey"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	// LogoURL is relative on purpose: it resolves against the page origin, so it
	// works through the Next.js rewrite proxy without the API ever needing to
	// know its own public hostname.
	LogoURL   *string   `json:"logoUrl"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Boundary struct {
	ID          string          `json:"id"`
	AreaKey     string          `json:"areaKey"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	ExternalRef *string         `json:"externalRef"`
	Geometry    json.RawMessage `json:"geometry"` // GeoJSON MultiPolygon
}

type Catalog struct {
	Areas      []Area     `json:"areas"`
	Categories []Category `json:"categories"`
}

// FacilityList is the paginated facilities response.
type FacilityList struct {
	Items []Facility `json:"items"`
	Total int        `json:"total"`
}

type BoundaryList struct {
	Items []Boundary `json:"items"`
}

// MapPayload is the single bootstrap call that replaces the old
// fetch('data/facility-mapping.kmz') round trip.
type MapPayload struct {
	Areas      []Area     `json:"areas"`
	Categories []Category `json:"categories"`
	Facilities []Facility `json:"facilities"`
	Boundaries []Boundary `json:"boundaries"`
}

type SessionUser struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	DisplayName *string `json:"displayName"`
	Role        string  `json:"role"`
}

type LogoUploaded struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	ContentType string `json:"contentType"`
	ByteSize    int    `json:"byteSize"`
}
