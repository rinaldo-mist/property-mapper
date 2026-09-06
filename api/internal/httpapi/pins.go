package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rinaldo-mist/property-mapper/api/internal/httpx"
	"github.com/rinaldo-mist/property-mapper/api/internal/store/gen"
)

// maxNameLength mirrors the maxlength="100" the old pin form enforced and the
// CHECK constraint on facilities.name.
const maxNameLength = 100

type createPinRequest struct {
	Name        string   `json:"name"`
	AreaKey     string   `json:"areaKey"`
	CategoryKey string   `json:"categoryKey"`
	Lat         *float64 `json:"lat"`
	Lng         *float64 `json:"lng"`
	LogoID      *string  `json:"logoId"`
}

func (s *Server) handleCreatePin(w http.ResponseWriter, r *http.Request) error {
	var req createPinRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	fields := map[string]string{}
	name := strings.TrimSpace(req.Name)
	validateName(name, fields)
	lat, lng := validateCoords(req.Lat, req.Lng, fields)
	if err := s.checkArea(r.Context(), req.AreaKey, fields); err != nil {
		return err
	}
	if err := s.checkCategory(r.Context(), req.CategoryKey, fields); err != nil {
		return err
	}
	logoID, err := parseOptionalUUID(req.LogoID, fields)
	if err != nil {
		return err
	}
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}

	user, _ := currentUser(r.Context())
	created, err := s.q.CreateManualPin(r.Context(), gen.CreateManualPinParams{
		Name:        name,
		AreaKey:     req.AreaKey,
		CategoryKey: req.CategoryKey,
		Lat:         lat,
		Lng:         lng,
		LogoID:      logoID,
		Actor:       user.ID,
	})
	if err != nil {
		return httpx.Internal(err)
	}
	return s.respondFacility(w, r, created, http.StatusCreated)
}

// updatePinRequest is a partial update.
//
// LogoID is json.RawMessage rather than *string so the three cases stay
// distinguishable: the key absent means "leave the logo alone", an explicit null
// means "remove it" (the "Hapus logo" button), and a string sets it. A *string
// would collapse the first two.
type updatePinRequest struct {
	Name        *string         `json:"name"`
	AreaKey     *string         `json:"areaKey"`
	CategoryKey *string         `json:"categoryKey"`
	Lat         *float64        `json:"lat"`
	Lng         *float64        `json:"lng"`
	LogoID      json.RawMessage `json:"logoId"`
}

func (s *Server) handleUpdatePin(w http.ResponseWriter, r *http.Request) error {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		return httpx.NotFound()
	}

	var req updatePinRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	fields := map[string]string{}
	params := gen.UpdateManualPinParams{ID: id}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		validateName(name, fields)
		params.Name = &name
	}
	if req.AreaKey != nil {
		if err := s.checkArea(r.Context(), *req.AreaKey, fields); err != nil {
			return err
		}
		params.AreaKey = req.AreaKey
	}
	if req.CategoryKey != nil {
		if err := s.checkCategory(r.Context(), *req.CategoryKey, fields); err != nil {
			return err
		}
		params.CategoryKey = req.CategoryKey
	}
	// Coordinates move together: updating one without the other would silently
	// place the pin somewhere neither the client nor the operator intended.
	if req.Lat != nil || req.Lng != nil {
		lat, lng := validateCoords(req.Lat, req.Lng, fields)
		params.Lat, params.Lng = &lat, &lng
	}

	switch {
	case len(req.LogoID) == 0: // key absent
	case string(req.LogoID) == "null":
		params.ClearLogo = true
	default:
		var raw string
		if err := json.Unmarshal(req.LogoID, &raw); err != nil {
			fields["logoId"] = "Logo tidak valid."
			break
		}
		logoID, err := parseOptionalUUID(&raw, fields)
		if err != nil {
			return err
		}
		params.LogoID = logoID
	}

	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}

	user, _ := currentUser(r.Context())
	params.Actor = user.ID

	updated, err := s.q.UpdateManualPin(r.Context(), params)
	if errors.Is(err, pgx.ErrNoRows) {
		// Either the pin does not exist, or it is an imported facility. The query
		// carries `AND source = 'manual'`, which is what preserves the old rule
		// that only hand-placed pins are editable.
		return httpx.NotFound()
	}
	if err != nil {
		return httpx.Internal(err)
	}
	return s.respondFacility(w, r, updated, http.StatusOK)
}

func (s *Server) handleDeletePin(w http.ResponseWriter, r *http.Request) error {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		return httpx.NotFound()
	}

	rows, err := s.q.DeleteManualPin(r.Context(), id)
	if err != nil {
		return httpx.Internal(err)
	}
	if rows == 0 {
		return httpx.NotFound()
	}
	httpx.NoContent(w)
	return nil
}

func (s *Server) respondFacility(w http.ResponseWriter, r *http.Request, id pgtype.UUID, status int) error {
	row, err := s.q.GetFacility(r.Context(), id)
	if err != nil {
		return httpx.Internal(err)
	}
	httpx.JSON(w, r, status, Facility{
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
	return nil
}

func validateName(name string, fields map[string]string) {
	switch {
	case name == "":
		fields["name"] = "Nama fasilitas wajib diisi."
	case len([]rune(name)) > maxNameLength:
		fields["name"] = "Nama fasilitas maksimal 100 karakter."
	}
}

func validateCoords(lat, lng *float64, fields map[string]string) (float64, float64) {
	if lat == nil || lng == nil {
		fields["lat"] = "Latitude dan longitude wajib diisi."
		return 0, 0
	}
	if math.IsNaN(*lat) || math.IsInf(*lat, 0) || *lat < -90 || *lat > 90 {
		fields["lat"] = "Latitude harus antara -90 dan 90."
	}
	if math.IsNaN(*lng) || math.IsInf(*lng, 0) || *lng < -180 || *lng > 180 {
		fields["lng"] = "Longitude harus antara -180 dan 180."
	}
	return *lat, *lng
}

// Foreign keys are checked up front so the client gets a field-keyed 422 naming
// the offending input, rather than an opaque 500 from a constraint violation.

func (s *Server) checkArea(ctx context.Context, key string, fields map[string]string) error {
	if key == "" {
		fields["areaKey"] = "Developer wajib dipilih."
		return nil
	}
	ok, err := s.q.AreaExists(ctx, key)
	if err != nil {
		return httpx.Internal(err)
	}
	if !ok {
		fields["areaKey"] = "Developer tidak dikenal."
	}
	return nil
}

func (s *Server) checkCategory(ctx context.Context, key string, fields map[string]string) error {
	if key == "" {
		fields["categoryKey"] = "Kategori wajib dipilih."
		return nil
	}
	ok, err := s.q.CategoryExists(ctx, key)
	if err != nil {
		return httpx.Internal(err)
	}
	if !ok {
		fields["categoryKey"] = "Kategori tidak dikenal."
	}
	return nil
}

func parseOptionalUUID(raw *string, fields map[string]string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if raw == nil || *raw == "" {
		return id, nil
	}
	parsed, err := parseUUID(*raw)
	if err != nil {
		fields["logoId"] = "Logo tidak valid."
		return id, nil
	}
	return parsed, nil
}
