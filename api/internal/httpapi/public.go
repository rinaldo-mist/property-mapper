package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rinaldo-mist/property-mapper/api/internal/httpx"
	"github.com/rinaldo-mist/property-mapper/api/internal/store/gen"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) error {
	httpx.JSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
	return nil
}

// handleReady is the Cloud Run startup probe: it reports ready only once the
// database is actually reachable, so traffic is not routed to an instance that
// would immediately 500.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := s.pool.Ping(ctx); err != nil {
		httpx.JSON(w, r, http.StatusServiceUnavailable, map[string]string{
			"status": "unavailable", "reason": "database",
		})
		return nil
	}
	httpx.JSON(w, r, http.StatusOK, map[string]string{"status": "ready"})
	return nil
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) error {
	catalog, err := s.catalog(r.Context())
	if err != nil {
		return err
	}
	httpx.JSON(w, r, http.StatusOK, catalog)
	return nil
}

func (s *Server) catalog(ctx context.Context) (*Catalog, error) {
	areaRows, err := s.q.ListAreas(ctx)
	if err != nil {
		return nil, httpx.Internal(err)
	}
	categoryRows, err := s.q.ListCategories(ctx)
	if err != nil {
		return nil, httpx.Internal(err)
	}

	areas := make([]Area, 0, len(areaRows))
	for _, a := range areaRows {
		areas = append(areas, Area{
			Key:           a.Key,
			Label:         a.Label,
			ShortCode:     a.ShortCode,
			SortOrder:     int(a.SortOrder),
			FacilityCount: int(a.FacilityCount),
		})
	}

	categories := make([]Category, 0, len(categoryRows))
	for _, c := range categoryRows {
		categories = append(categories, Category{
			Key:           c.Key,
			Label:         c.Label,
			Color:         c.Color,
			Icon:          c.Icon,
			SortOrder:     int(c.SortOrder),
			IsFallback:    c.IsFallback,
			FacilityCount: int(c.FacilityCount),
		})
	}
	return &Catalog{Areas: areas, Categories: categories}, nil
}

func (s *Server) handleListFacilities(w http.ResponseWriter, r *http.Request) error {
	params, err := parseFacilityFilters(r)
	if err != nil {
		return err
	}
	items, total, err := s.facilities(r.Context(), params)
	if err != nil {
		return err
	}
	httpx.JSON(w, r, http.StatusOK, FacilityList{Items: items, Total: total})
	return nil
}

func (s *Server) facilities(ctx context.Context, params gen.ListFacilitiesParams) ([]Facility, int, error) {
	rows, err := s.q.ListFacilities(ctx, params)
	if err != nil {
		return nil, 0, httpx.Internal(err)
	}

	items := make([]Facility, 0, len(rows))
	total := 0
	for _, row := range rows {
		total = int(row.TotalCount) // identical on every row: count(*) OVER ()
		items = append(items, Facility{
			ID:          uuidString(row.ID),
			Source:      string(row.Source),
			Name:        row.Name,
			AreaKey:     row.AreaKey,
			CategoryKey: row.CategoryKey,
			Lat:         row.Lat,
			Lng:         row.Lng,
			LogoURL:     logoURL(row.LogoID),
			UpdatedAt:   row.UpdatedAt.Time,
		})
	}
	return items, total, nil
}

func (s *Server) handleListBoundaries(w http.ResponseWriter, r *http.Request) error {
	var areaKey *string
	if v := strings.TrimSpace(r.URL.Query().Get("area")); v != "" {
		areaKey = &v
	}
	items, err := s.boundaries(r.Context(), areaKey)
	if err != nil {
		return err
	}
	httpx.JSON(w, r, http.StatusOK, BoundaryList{Items: items})
	return nil
}

func (s *Server) boundaries(ctx context.Context, areaKey *string) ([]Boundary, error) {
	rows, err := s.q.ListBoundaries(ctx, areaKey)
	if err != nil {
		return nil, httpx.Internal(err)
	}
	items := make([]Boundary, 0, len(rows))
	for _, row := range rows {
		items = append(items, Boundary{
			ID:          uuidString(row.ID),
			AreaKey:     row.AreaKey,
			Name:        row.Name,
			Description: row.Description,
			ExternalRef: row.ExternalRef,
			// Already GeoJSON from ST_AsGeoJSON; forwarded verbatim rather than
			// decoded and re-encoded.
			Geometry: json.RawMessage(row.Geometry),
		})
	}
	return items, nil
}

// handleMap is the single bootstrap call. It replaces the KMZ download, the
// client-side unzip and the DOM parse the old app did on every page load.
func (s *Server) handleMap(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	catalog, err := s.catalog(ctx)
	if err != nil {
		return err
	}
	facilities, _, err := s.facilities(ctx, gen.ListFacilitiesParams{CategoryKeys: []string{}})
	if err != nil {
		return err
	}
	boundaries, err := s.boundaries(ctx, nil)
	if err != nil {
		return err
	}

	httpx.JSON(w, r, http.StatusOK, MapPayload{
		Areas:      catalog.Areas,
		Categories: catalog.Categories,
		Facilities: facilities,
		Boundaries: boundaries,
	})
	return nil
}

// parseFacilityFilters maps query parameters onto the generated params struct.
// Every filter is optional; an absent one leaves its clause disabled.
func parseFacilityFilters(r *http.Request) (gen.ListFacilitiesParams, error) {
	query := r.URL.Query()
	params := gen.ListFacilitiesParams{CategoryKeys: []string{}}

	if v := strings.TrimSpace(query.Get("area")); v != "" {
		params.AreaKey = &v
	}
	for _, c := range query["category"] {
		if c = strings.TrimSpace(c); c != "" {
			params.CategoryKeys = append(params.CategoryKeys, c)
		}
	}
	if v := strings.TrimSpace(query.Get("q")); v != "" {
		// Escape LIKE wildcards so a search for "100%" is a literal search.
		escaped := strings.NewReplacer("%", `\%`, "_", `\_`, `\`, `\\`).Replace(v)
		params.Query = &escaped
	}

	if raw := strings.TrimSpace(query.Get("bbox")); raw != "" {
		parts := strings.Split(raw, ",")
		if len(parts) != 4 {
			return params, httpx.BadRequest("Parameter bbox harus berisi empat angka: minLng,minLat,maxLng,maxLat.")
		}
		coords := make([]float64, 4)
		for i, p := range parts {
			f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
			if err != nil {
				return params, httpx.BadRequest("Parameter bbox berisi angka yang tidak valid.")
			}
			coords[i] = f
		}
		params.MinLng, params.MinLat = &coords[0], &coords[1]
		params.MaxLng, params.MaxLat = &coords[2], &coords[3]
	}

	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 5000 {
			return params, httpx.BadRequest("Parameter limit harus antara 1 dan 5000.")
		}
		v := int32(n)
		params.Limit = &v
	}
	if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return params, httpx.BadRequest("Parameter offset tidak valid.")
		}
		v := int32(n)
		params.Offset = &v
	}
	return params, nil
}

// logoURL returns a RELATIVE URL. Relative matters: it resolves against the page
// origin, so it works through the rewrite proxy without the API knowing its own
// public hostname.
func logoURL(id pgtype.UUID) *string {
	if !id.Valid {
		return nil
	}
	url := "/api/v1/logos/" + uuidString(id)
	return &url
}
